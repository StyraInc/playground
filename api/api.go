package api

import (
	"bufio"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"path"
	"reflect"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"github.com/google/go-github/v73/github"
	"github.com/gorilla/mux"
	"github.com/mattbaird/jsonpatch"
	"github.com/open-policy-agent/opa/ast"
	"github.com/open-policy-agent/opa/bundle"
	coverpkg "github.com/open-policy-agent/opa/cover"
	"github.com/open-policy-agent/opa/format"
	"github.com/open-policy-agent/opa/rego"
	"github.com/open-policy-agent/opa/topdown"
	"github.com/open-policy-agent/opa/util"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	log "github.com/sirupsen/logrus"
	"github.com/styrainc/regal/pkg/config"
	"github.com/styrainc/regal/pkg/linter"
	"github.com/styrainc/regal/pkg/report"
	"github.com/styrainc/regal/pkg/rules"
	"golang.org/x/oauth2"
	oauthGithub "golang.org/x/oauth2/github"

	"github.com/open-policy-agent/rego-playground/opa"
	"github.com/open-policy-agent/rego-playground/presentation"
	"github.com/open-policy-agent/rego-playground/version"
)

type apiError struct {
	Code    string      `json:"code"`
	Message string      `json:"message,omitempty"`
	Error   interface{} `json:"error,omitempty"` // Error or collection of errors from Rego parsing / compilation / evaluation
	Trace   interface{} `json:"trace,omitempty"`
	Ignored []string    `json:"ignored,omitempty"`
}

type apiRouteNotFoundError struct {
	Error struct {
		Code string `json:"code"`
	} `json:"error"`
}

// DataRequest represents the data received from the FE
type DataRequest struct {
	Input               *interface{}                    `json:"input"`                  // (optional)
	Data                *interface{}                    `json:"data"`                   // (optional)
	RegoModules         map[string]interface{}          `json:"rego_modules"`           // (optional) typically will contain at least one module
	RegoQuery           string                          `json:"rego"`                   // (optional) client side; if empty, opa.go will query either data.<package> or data depending on whether there's one or more modules
	RegoVersion         *int                            `json:"rego_version"`           // (optional) version of Rego to parse for
	QueryPackage        *string                         `json:"query_package"`          // (optional) set the package for the query. Nil with more than one module or a non-nil pointer to an empty string indicates that the query should not have a package.
	QueryImports        *[]string                       `json:"query_imports"`          // (optional) set the imports (e.g. "foo.bar" or "foo.bar as baz") for the query. Nil with more than one module or a non-nil pointer to an empty list indicates that the query should not have any imports.
	Trace               bool                            `json:"trace"`                  // (optional) tracing should be enabled (used for watch behaviour)
	Coverage            bool                            `json:"coverage"`               // (optional) coverage should be enabled during evaluation
	Strict              bool                            `json:"strict"`                 // (optional) compiler strict-mode should be enabled
	ReadOnly            bool                            `json:"read_only"`              // (optional)
	BuiltInErrorsAll    bool                            `json:"built_in_errors_all"`    // (optional) if true, all built-in errors will be returned
	BuiltInErrorsStrict bool                            `json:"built_in_errors_strict"` // (optional) if true, the first built in error encountered is fatal returned
	Etag                string                          `json:"etag"`                   // (optional)
	Patch               *[]jsonpatch.JsonPatchOperation `json:"patch"`
}

// DataRequestStore represents a system for storing and retrieving DataRequests.
type DataRequestStore interface {
	// Get a DataRequest from the store (potentially empty) and whether that key is set, returns an error if the correct values for both could not be determined (e.g. errors from the network, demarshaling, etc...).
	Get(key *StoreKey, principal *Principal) (DataRequest, bool, error)
	// Put adds/sets a DataRequest in the store, or errors if it can't.
	Put(key *StoreKey, dr DataRequest, principal *Principal) (*StoreKey, error)
	// List the keys corresponding to DataRequests that can be gotten. Keys can be filtered using a prefix ("" for all).
	List(prefix *StoreKey, principal *Principal) ([]*StoreKey, error)
	// ListAll coresponds to List with an empty prefix.
	// deprecated
	ListAll(principal *Principal) ([]*StoreKey, error)
	// Watch adds a watcher to provide change notifications when the store is changed.
	Watch(key *StoreKey, etag string, timeout time.Duration, cb func(DataRequest), principal *Principal) (bool, error)
}

type KeyType int

const (
	KeyTypeNone   KeyType = iota
	KeyTypeLegacy         // FIXME: Should we rename these to Raw/Json instead of Legacy/Gist?
	KeyTypeGist
)

type StoreKey struct {
	Id       string  `json:"id"`
	Revision string  `json:"revision,omitempty"`
	KeyType  KeyType `json:"-"`
}

func (sk *StoreKey) withoutRevision() *StoreKey {
	if sk == nil {
		return nil
	}
	return &StoreKey{
		Id: sk.Id,
	}
}

func (sk *StoreKey) toOpaque() (string, error) {
	if sk == nil {
		return "", errors.New("store key is nil")
	}

	if sk.KeyType == KeyTypeNone || sk.KeyType == KeyTypeLegacy {
		return sk.Id, nil
	}

	decodedBytes, err := hex.DecodeString(sk.Revision)
	if err != nil {
		log.Fatalf("Error decoding hex string: %v", err)
	}

	b64 := base64.RawURLEncoding.EncodeToString([]byte(sk.Id + "_" + string(decodedBytes)))

	return fmt.Sprintf("g_%s", b64), nil
}

func (sk *StoreKey) String() string {
	if sk == nil {
		return "<nil>"
	}

	buf := &strings.Builder{}
	buf.WriteString("<")
	buf.WriteString(sk.Id)
	if sk.Revision != "" {
		buf.WriteString(":")
		buf.WriteString(sk.Revision)
	}
	buf.WriteString(">")

	return buf.String()
}

func storeKeyFromOpaque(opaque string) (*StoreKey, error) {
	if !strings.HasPrefix(opaque, "g_") {
		return &StoreKey{Id: opaque, KeyType: KeyTypeLegacy}, nil
	}

	b64 := strings.TrimPrefix(opaque, "g_")
	j, err := base64.RawURLEncoding.DecodeString(b64)
	if err != nil {
		return nil, fmt.Errorf("failed to decode share ID: %w", err)
	}

	before, after, found := strings.Cut(string(j), "_")
	if !found {
		return nil, fmt.Errorf("failed to decode share ID: %w", err)
	}

	var sd StoreKey
	sd.Id = before
	sd.Revision = hex.EncodeToString([]byte(after))

	sd.KeyType = KeyTypeGist

	return &sd, nil
}

type Principal struct {
	oauthConfig *oauth2.Config
	accessToken *oauth2.Token
}

func (p *Principal) AccessToken() *oauth2.Token {
	return p.accessToken
}

func (p *Principal) Client(ctx context.Context) *github.Client {
	return github.NewClient(p.oauthConfig.Client(ctx, p.accessToken))
}

// DataResponse represents the data returned to the FE
type DataResponse struct {
	Result        interface{}      `json:"result"`
	BundleId      interface{}      `json:"bundle_id"`
	BundleUrl     interface{}      `json:"bundle_url"`
	CommitId      interface{}      `json:"commit_id"`
	CommitUrl     interface{}      `json:"commit_url"`
	Pretty        interface{}      `json:"pretty"` // The "pretty"-printed results
	Value         string           `json:"value"`
	Input         *interface{}     `json:"input"`
	Data          *interface{}     `json:"data"`
	RegoVersion   *int             `json:"rego_version"`
	EvalTime      interface{}      `json:"eval_time"`
	BuiltInErrors []topdown.Error  `json:"built_in_errors,omitempty"`
	Trace         interface{}      `json:"trace,omitempty"`
	Output        string           `json:"output,omitempty"`
	Coverage      *coverpkg.Report `json:"coverage,omitempty"`
	Ignored       []string         `json:"ignored,omitempty"`
}

// InputResponse represents a policy's input
type InputResponse struct {
	Input *interface{} `json:"input"`
}

// VersionResponse represents versions
type VersionResponse struct {
	OPAVersion        string `json:"opa_version"`
	OPAReleaseVersion string `json:"opa_release_version"`
	PlayGroundVersion string `json:"playground_version"`
	PlayGroundVCS     string `json:"playground_vcs"`
	RegalVersion      string `json:"regal_version"`
}

// VarsRequest represents rego selection eval
type VarsRequest struct {
	RegoModule    string `json:"rego_module"`
	RegoSelection string `json:"rego_selection"`
}

// VarsResponse represents vars
type VarsResponse struct {
	Result []string `json:"result"`
}

type FormatRequest struct {
	RegoVersion *int   `json:"rego_version"`
	RegoModule  string `json:"rego_module"`
}

// FormatResponse respresents module formatting result
type FormatResponse struct {
	Result      string      `json:"result"`
	FormatTime  interface{} `json:"fmt_time"`
	RegoVersion int         `json:"rego_version"`
}

// LintRequest represents a request from the frontend to lint rego code
type LintRequest struct {
	RegoVersion *int   `json:"rego_version"`
	RegoModule  string `json:"rego_module"`
}

// LintResponse represents a response to a lint request
type LintResponse struct {
	ErrorMessage string         `json:"error_message"`
	Errors       []*ast.Error   `json:"errors"`
	Report       *report.Report `json:"report"`
}

type update struct {
	cb   func(DataRequest)
	done chan struct{}
}

const (
	apiCodeNotFound         = "not_found"
	apiCodeParseError       = "parse_error"
	apiCodeForbidden        = "forbidden"
	apiCodeUnauthorized     = "unauthorized"
	apiCodeInternalError    = "internal_error"
	apiCodeFileTooLarge     = "file_too_large"
	apiCodeInvalidArgument  = "invalid_argument"
	maxUploadSizeLimitBytes = int64(32768) // 32KB size limit

	// Set of handlers for use in the "handler" dimension of the duration metric.
	promHandlerBundlesGet       = "v1/bundles_get"
	promHandlerV1Data           = "v1/data"
	promHandlerV1ShareGet       = "v1/share_get"
	promHandlerV1SharePost      = "v1/share_post"
	promHandlerV1VarsPost       = "v1/vars_post"
	promHandlerV1Lint           = "v1/lint"
	promHandlerV1FormattingPost = "v1/formatting_post"
	promHandlerV1CORSPreflight  = "v1/cors_preflight"

	corsMaxAgeSec = "7200" // How long to let browsers cache CORS preflight responses. 2 hours is chromium's default (after v76)

	// deltaBundleMode indicates that OPA supports delta bundle processing
	deltaBundleMode = "delta"

	// defaultBundleMode indicates that OPA supports snapshot bundle processing
	defaultBundleMode = "snapshot"

	githubAuthCookie = "github_access_token"
)

var (
	seededRand = rand.New(rand.NewSource(time.Now().UnixNano()))
	letter     = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789")
)

type Auth interface {
	Exchange(ctx context.Context, code string) (*oauth2.Token, error)
	Token(context.Context, *oauth2.Token) (*oauth2.Token, error)
	Check(context.Context, *oauth2.Token) bool
}

type GithubAuth struct {
	config *oauth2.Config
}

func NewGithubAuth(config *oauth2.Config) *GithubAuth {
	return &GithubAuth{
		config: config,
	}
}

// Token returns a token (automatically refreshed if possible) or an error
func (g GithubAuth) Token(ctx context.Context, token *oauth2.Token) (*oauth2.Token, error) {
	return g.config.TokenSource(ctx, token).Token()
}

// Exchange converts an authorization code into a token
func (g GithubAuth) Exchange(ctx context.Context, code string) (*oauth2.Token, error) {
	return g.config.Exchange(ctx, code)
}

func (g GithubAuth) Check(ctx context.Context, token *oauth2.Token) bool {
	client := github.NewClient(g.config.Client(ctx, token))
	_, _, err := client.Gists.ListAll(ctx, nil)
	if err != nil {
		return false
	}

	return true
}

// API implements a simple HTTP API server.
type API struct {
	addr              string
	router            *mux.Router
	v1Store           DataRequestStore
	v2Store           DataRequestStore
	contentRoot       string
	externalURL       string
	githubOauthConfig *oauth2.Config
	auth              Auth
}

// NewAPIService returns a instance of the API.
func NewAPIService(addr string, v1Store, v2Store DataRequestStore, contentRoot string, externalURL string, githubClientID string, githubClientSecret string) *API {
	redirectURL := fmt.Sprintf("%s/v1/githubcallback", strings.TrimSuffix(externalURL, "/"))
	log.Debugf("Redirect URL %s", redirectURL)

	conf := &oauth2.Config{
		ClientID:     githubClientID,
		ClientSecret: githubClientSecret,
		RedirectURL:  redirectURL,
		Endpoint:     oauthGithub.Endpoint,
		Scopes:       []string{"gist"},
	}

	api := &API{
		addr:              addr,
		v1Store:           v1Store,
		v2Store:           v2Store,
		contentRoot:       contentRoot,
		externalURL:       externalURL,
		githubOauthConfig: conf,
		auth:              NewGithubAuth(conf),
	}

	api.router = mux.NewRouter()

	promRegistry := prometheus.NewRegistry()
	duration := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name: "http_request_duration_seconds",
			Help: "A histogram of duration for requests.",
		},
		[]string{"code", "handler", "method"},
	)
	v1BundlesGetDur := duration.MustCurryWith(prometheus.Labels{"handler": promHandlerBundlesGet})
	v1DataDur := duration.MustCurryWith(prometheus.Labels{"handler": promHandlerV1Data})
	v1ShareGetDur := duration.MustCurryWith(prometheus.Labels{"handler": promHandlerV1ShareGet})
	v1SharePostDur := duration.MustCurryWith(prometheus.Labels{"handler": promHandlerV1SharePost})
	v1LintDur := duration.MustCurryWith(prometheus.Labels{"handler": promHandlerV1Lint})
	v1Vars := duration.MustCurryWith(prometheus.Labels{"handler": promHandlerV1VarsPost})
	v1Formatting := duration.MustCurryWith(prometheus.Labels{"handler": promHandlerV1FormattingPost})
	v1CORSPreflightDur := duration.MustCurryWith(prometheus.Labels{"handler": promHandlerV1CORSPreflight})
	promRegistry.MustRegister(duration)
	promRegistry.MustRegister(prometheus.NewGoCollector())

	api.router.StrictSlash(true)
	api.router.Handle("/metrics", promhttp.HandlerFor(promRegistry, promhttp.HandlerOpts{})).Methods(http.MethodGet)
	api.router.HandleFunc("/bundles/{key}", promhttp.InstrumentHandlerDuration(v1BundlesGetDur, http.HandlerFunc(api.handleRetrieveBundle))).Methods(http.MethodGet)
	api.router.HandleFunc("/v1/input/{key}", promhttp.InstrumentHandlerDuration(v1ShareGetDur, http.HandlerFunc(api.handleRetrieveInput))).Methods(http.MethodGet)
	api.router.HandleFunc("/v1/data", promhttp.InstrumentHandlerDuration(v1DataDur, http.HandlerFunc(api.handleQuery))).Methods(http.MethodPost)
	api.router.HandleFunc("/v1/data/{path:.+}", promhttp.InstrumentHandlerDuration(v1DataDur, http.HandlerFunc(api.handleQuery))).Methods(http.MethodPost)
	api.router.HandleFunc("/v1/data/{key:.+}", promhttp.InstrumentHandlerDuration(v1ShareGetDur, http.HandlerFunc(api.handleRetrieveFromStore))).Methods(http.MethodGet)
	api.router.HandleFunc("/v1/distribute", promhttp.InstrumentHandlerDuration(v1SharePostDur, http.HandlerFunc(api.handleCreateDistribute))).Methods(http.MethodPost)
	api.router.HandleFunc("/v1/distribute/{key}", promhttp.InstrumentHandlerDuration(v1SharePostDur, http.HandlerFunc(api.handleUpdateDistribute))).Methods(http.MethodPut)
	api.router.HandleFunc("/v1/share", promhttp.InstrumentHandlerDuration(v1SharePostDur, http.HandlerFunc(api.handleShareUpload))).Methods(http.MethodPost)
	api.router.HandleFunc("/v2/decode/{key:.+}", api.decodeKey).Methods(http.MethodGet)
	api.router.HandleFunc("/v2/publish", promhttp.InstrumentHandlerDuration(v1SharePostDur, http.HandlerFunc(api.handlePublish))).Methods(http.MethodPost)
	api.router.HandleFunc("/v2/publish/{key}", promhttp.InstrumentHandlerDuration(v1SharePostDur, http.HandlerFunc(api.handleUpdatePublish))).Methods(http.MethodPut)
	api.router.HandleFunc("/v2/auth/test", api.testAuth)
	api.router.HandleFunc("/v2/auth", api.handleGithubAuth)
	api.router.HandleFunc("/v1/githubcallback", api.handleGithubCallback) // TODO rename this to /v2/authcallback
	api.router.HandleFunc("/v1/session", api.handleSession).Methods(http.MethodGet)
	api.router.HandleFunc("/v1/lint", promhttp.InstrumentHandlerDuration(v1LintDur, http.HandlerFunc(api.handleLint))).Methods(http.MethodPost)
	api.router.HandleFunc("/v1/system/alive", api.handleLiveness).Methods(http.MethodGet)
	api.router.HandleFunc("/v1/system/ready", api.handleReadiness).Methods(http.MethodGet)
	api.router.HandleFunc("/v1/fmt", promhttp.InstrumentHandlerDuration(v1Formatting, http.HandlerFunc(api.handleFormatting))).Methods(http.MethodPost)
	api.router.HandleFunc("/v1/vars", promhttp.InstrumentHandlerDuration(v1Vars, http.HandlerFunc(api.handleVars))).Methods(http.MethodPost)
	api.router.HandleFunc("/version", api.handleVersion).Methods(http.MethodGet)
	api.router.HandleFunc("/experimental", api.createNewExperimentalCookie).Methods(http.MethodGet)

	// Preflight handlers for routes that can be accessed outside play.openpolicyagent.org. The real handlers for these routes also need to call addCORSHeaders(w, r).
	api.router.HandleFunc("/v1/data", promhttp.InstrumentHandlerDuration(v1CORSPreflightDur, http.HandlerFunc(api.handleCORSPreflight))).Methods(http.MethodOptions)
	api.router.HandleFunc("/v1/share", promhttp.InstrumentHandlerDuration(v1CORSPreflightDur, http.HandlerFunc(api.handleCORSPreflight))).Methods(http.MethodOptions)

	api.router.NotFoundHandler = http.HandlerFunc(api.handleNotFound)

	// Serve the frontend content directory at `/` and `/p` and `/play` and `/d` and `/distribute`
	api.router.HandleFunc("/distribute/{key}", api.handleServeShared).Methods(http.MethodGet)
	api.router.HandleFunc("/d/{key}", api.handleServeShared).Methods(http.MethodGet)
	api.router.HandleFunc("/play/{key}", api.handleServeShared).Methods(http.MethodGet)
	api.router.HandleFunc("/p/{key}", api.handleServeShared).Methods(http.MethodGet)

	gzipHandler := &gzipHandler{
		handler: http.FileServer(http.Dir(contentRoot)),
	}
	api.router.PathPrefix("/").Handler(gzipHandler).Methods(http.MethodGet)

	return api
}

// Init initializes the service.
func (api *API) Init(ctx context.Context) error {
	log.ErrorKey = "err"
	return nil
}

// Start starts the HTTP server.
func (api *API) Start(ctx context.Context) error {
	go api.run()
	return nil
}

// Stop stops the HTTP server. // TODO(sr): Does it?
func (*API) Stop(context.Context) error {
	return nil
}

func (api *API) run() error {
	log.WithField("addr", api.addr).Info("Starting Rego Playground server...")
	return http.ListenAndServe(api.addr, RecoveryHandler()(api.router))
}

func (api *API) handleNotFound(w http.ResponseWriter, r *http.Request) {
	var resp apiRouteNotFoundError
	resp.Error.Code = apiCodeNotFound
	writeJSON(w, http.StatusNotFound, resp)
}

// createNewExperimentalCookie creates a cookie to act as a feature flag for experimental features
func (api *API) createNewExperimentalCookie(w http.ResponseWriter, r *http.Request) {
	name := "experimental"
	cookie := &http.Cookie{
		Name:  name,
		Value: "true",
		Path:  "/",
	}

	existingCookie, _ := r.Cookie(name)
	if existingCookie != nil {
		cookie.Expires = time.Now().Add(-7 * time.Hour)
	}

	http.SetCookie(w, cookie)
	http.Redirect(w, r, "/", http.StatusFound)
}

// Some requests from origins other than play.openpolicyagent.org will result in the browser "preflighting" the request (sending a preliminary request with the OPTIONS method, https://developer.mozilla.org/en-US/docs/Web/HTTP/CORS#Preflighted_requests).
// In particular, this will happen when the Content-Type header is correctly set to 'application/json'.
func (api *API) handleCORSPreflight(w http.ResponseWriter, r *http.Request) {
	addCORSHeaders(w, r)
	w.WriteHeader(200)
}

func (api *API) handleQuery(w http.ResponseWriter, r *http.Request) {
	api.doHandleQuery(w, r)
}

func (api *API) doHandleQuery(w http.ResponseWriter, r *http.Request) {
	addCORSHeaders(w, r)

	bs, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusInternalServerError, apiCodeInternalError, fmt.Errorf("failed reading request body: %w", err))
		return
	}

	var msg DataRequest

	err = util.UnmarshalJSON(bs, &msg)
	if err != nil {
		writeError(w, http.StatusBadRequest, apiCodeParseError, err)
		return
	}

	if len(msg.RegoModules) == 0 {
		writeError(w, http.StatusBadRequest, apiCodeParseError, err)
		return
	}

	// Map paths and values to the filename and a string containing the module.
	// Rego module sample key format: <package_name>/<policy_file_name>
	// eg. rbac/authz/authz.rego
	policies := make(map[string]string)
	for path, value := range msg.RegoModules {
		parts := strings.Split(path, "/")
		str, ok := value.(string)
		if !ok {
			writeError(w, http.StatusBadRequest, apiCodeParseError, err)
		}
		policies[parts[len(parts)-1]] = str
	}

	fields := log.Fields{
		"input":    nil,
		"policies": policies,
		"query":    msg.RegoQuery,
	}

	if msg.Input != nil {
		fields["input"] = *msg.Input
	}
	if msg.QueryPackage != nil {
		fields["queryPackage"] = *msg.QueryPackage
	}

	if msg.QueryImports != nil {
		fields["queryImports"] = *msg.QueryImports
	}

	if msg.Data != nil {
		fields["data"] = *msg.Data
	}

	// disable strict mode to allow valid queries to be compiled
	if msg.RegoQuery != "" {
		msg.Strict = false
	}

	log.WithFields(fields).Debug("Input to OPA.")

	compileWithVersion := func(version int) (*opa.CompileResult, opa.Ignored, error) {
		return opa.Compile(
			r.Context(),
			msg.Input, msg.Data,
			policies,
			msg.RegoQuery, msg.QueryPackage, msg.QueryImports, msg.Strict,
			&version,
		)
	}

	regoVersion := 1
	if msg.RegoVersion != nil {
		regoVersion = *msg.RegoVersion
	}

	compileResult, ignored, err := compileWithVersion(regoVersion)
	if err != nil {
		if regoVersion == 1 {
			// if there is an error parsing, and we were using v1, then attempt to parse as v0
			compileResultv0, ignoredv0, errv0 := compileWithVersion(0)
			// only if there is no err from the v0 operation, should the results be adopted
			if errv0 == nil {
				err, compileResult, ignored = errv0, compileResultv0, ignoredv0

				// update the regoVersion here so the client can adapt and warn the user
				regoVersion = 0
			}
		}
		if err != nil {
			log.WithError(err).WithField("version", regoVersion).Error("Compile Error")
			writeErrorAndIgnored(w, http.StatusBadRequest, apiCodeParseError, err, ignored)
			return
		}
	}

	result, evalErr := opa.Eval(
		r.Context(),
		compileResult,
		opa.EvalOptions{
			DebugTrace:          msg.Trace,
			Cover:               msg.Coverage,
			BuiltInErrorsAll:    msg.BuiltInErrorsAll,
			BuiltInErrorsStrict: msg.BuiltInErrorsStrict,
		},
	)
	if evalErr != nil {
		log.WithError(evalErr.RawError).Error("Eval Error.")
		writeErrorAndIgnored(w, evalErr.HTTPStatus, apiCodeInternalError, evalErr.RawError, ignored)
		return
	}
	response := DataResponse{
		Result:   result.Result,
		EvalTime: result.Time,
		Pretty:   presentation.PrettyResultString(result.Result),
		Trace:    result.Trace,
		Output:   result.Output,
		// this is used in the UI to test if the version used was different
		// from the one supplied, trigger warnings etc.
		RegoVersion: &regoVersion,
	}
	if msg.Trace {
		response.Trace = result.Trace
	}
	if msg.Coverage {
		response.Coverage = result.Coverage
	}
	response.Ignored = ignored
	writeJSON(w, http.StatusOK, response)
}

func (api *API) handleCreateDistribute(w http.ResponseWriter, r *http.Request) {
	addCORSHeaders(w, r)
	bs, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, apiCodeParseError, err)
		return
	}

	var msg DataRequest
	if err := util.UnmarshalJSON(bs, &msg); err != nil {
		writeError(w, http.StatusBadRequest, apiCodeParseError, err)
		return
	}

	if int64(len(bs)) > maxUploadSizeLimitBytes {
		errMsg := fmt.Errorf("cannot distribute files greater than %v bytes", maxUploadSizeLimitBytes)
		writeError(w, http.StatusBadRequest, apiCodeFileTooLarge, errMsg)
		return
	}

	msg.ReadOnly = false

	bs, _ = json.Marshal(msg)
	etag, err := getEtag(bs)
	if err != nil {
		writeError(w, http.StatusInternalServerError, apiCodeInternalError, err)
		return
	}

	msg.Etag = etag

	key := &StoreKey{Id: getUniqueID(), KeyType: KeyTypeLegacy}
	key, err = api.v1Store.Put(key, msg, api.getPrincipal(r))
	if err != nil {
		writeError(w, http.StatusInternalServerError, apiCodeInternalError, err)
		return
	}

	log.Debugf("Data uploaded successfully. Key: %v, Data: %+v", key, msg)

	oKey, err := key.toOpaque()
	if err != nil {
		writeError(w, http.StatusInternalServerError, apiCodeInternalError, fmt.Errorf("failed to create opaque key: %w", err))
		return
	}

	result := DataResponse{
		Result: fmt.Sprintf("%s/d/%s", strings.TrimSuffix(api.externalURL, "/"), oKey),
	}
	writeJSON(w, http.StatusOK, result)
}

func (api *API) handleUpdateDistribute(w http.ResponseWriter, r *http.Request) {
	addCORSHeaders(w, r)
	key := getKeyFromRequest(r)

	api.doHandleUpdateDistribute(w, r, key)
}

func (api *API) doHandleUpdateDistribute(w http.ResponseWriter, r *http.Request, key *StoreKey) {
	bs, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, apiCodeParseError, err)
		return
	}

	var msg DataRequest
	if err := util.UnmarshalJSON(bs, &msg); err != nil {
		writeError(w, http.StatusBadRequest, apiCodeParseError, err)
		return
	}

	if int64(len(bs)) > maxUploadSizeLimitBytes {
		errMsg := fmt.Errorf("cannot distribute files greater than %v bytes", maxUploadSizeLimitBytes)
		writeError(w, http.StatusBadRequest, apiCodeFileTooLarge, errMsg)
		return
	}

	bs, _ = json.Marshal(msg)
	etag, err := getEtag(bs)
	if err != nil {
		writeError(w, http.StatusInternalServerError, apiCodeInternalError, err)
		return
	}

	msg.Etag = etag

	// this is a v1 distribution call, only check the v1 store
	existingDataReq, found, err := api.v1Store.Get(key, api.getPrincipal(r))
	if err != nil {
		if errors.Is(err, &UnauthorizedError{}) {
			writeError(w, http.StatusUnauthorized, apiCodeUnauthorized, err)
			return
		}

		writeError(w, http.StatusInternalServerError, apiCodeInternalError, err)
		return
	}
	if !found {
		writeError(w, http.StatusNotFound, apiCodeNotFound, err)
		return
	}

	policyUpdated := false
	if !reflect.DeepEqual(msg.RegoModules, existingDataReq.RegoModules) {
		policyUpdated = true
		msg.Patch = nil
	}

	// create a JSON Patch if there is a data change but no policy update
	if !policyUpdated {
		err := generateJSONDataPatch(&existingDataReq, &msg)
		if err != nil {
			writeError(w, http.StatusInternalServerError, apiCodeInternalError, err)
			return
		}
	}

	key, err = api.v1Store.Put(key, msg, api.getPrincipal(r))
	if err != nil {
		writeError(w, http.StatusInternalServerError, apiCodeInternalError, err)
		return
	}

	if msg.ReadOnly {
		writeError(w, http.StatusForbidden, apiCodeInvalidArgument, errors.New("cannot update readonly resource"))
		return
	}

	log.Debugf("Data uploaded successfully. Key: %v, Data: %+v", key, msg)

	result := DataResponse{
		Result: fmt.Sprintf("%s/d/%s", strings.TrimSuffix(api.externalURL, "/"), key.Id),
	}
	writeJSON(w, http.StatusOK, result)
}

func (api *API) handleLint(w http.ResponseWriter, r *http.Request) {
	addCORSHeaders(w, r)

	response := LintResponse{
		Errors: []*ast.Error{},
	}

	bs, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusInternalServerError, apiCodeInternalError, err)
		return
	}

	var req LintRequest
	if err := util.UnmarshalJSON(bs, &req); err != nil {
		writeError(w, http.StatusBadRequest, apiCodeInvalidArgument, err)
		return
	}

	input, err := rules.InputFromTextWithOptions(
		"policy.rego",
		req.RegoModule,
		ast.ParserOptions{RegoVersion: regoVersionFromRequest(req.RegoVersion, ast.RegoUndefined)},
	)
	if err != nil {
		var astErrs ast.Errors
		var astErr *ast.Error
		response.ErrorMessage = err.Error()
		if errors.As(err, &astErrs) {
			response.ErrorMessage = astErrs.Error()
			response.Errors = astErrs
		} else if errors.As(err, &astErr) {
			response.ErrorMessage = astErr.Message
			response.Errors = ast.Errors{astErr}
		}

		writeJSON(w, http.StatusOK, response)
		return
	}

	regalInstance := linter.NewLinter().
		WithInputModules(&input).
		WithUserConfig(config.Config{
			Rules: map[string]config.Category{
				"idiomatic": {
					"directory-package-mismatch": config.Rule{
						// this rule is disabled because the playground
						// operates with out the notion of a directory.
						Level: "ignore",
					},
				},
				"style": {
					"line-length": config.Rule{
						Extra: map[string]interface{}{
							// this allows some long tokens to appear in example
							// header comments without breaking the line length rule
							"non-breakable-word-threshold": 100,
						},
					},
				},
			},
		})

	rpt, err := regalInstance.Lint(r.Context())
	if err != nil {
		response.ErrorMessage = err.Error()
		writeJSON(w, http.StatusOK, response)
		return
	}

	response.Report = &rpt

	writeJSON(w, http.StatusOK, response)
}

func (api *API) handleShareUpload(w http.ResponseWriter, r *http.Request) {
	addCORSHeaders(w, r)
	bs, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, apiCodeParseError, err)
		return
	}

	var msg DataRequest
	if err := util.UnmarshalJSON(bs, &msg); err != nil {
		writeError(w, http.StatusBadRequest, apiCodeParseError, err)
		return
	}

	if int64(len(bs)) > maxUploadSizeLimitBytes {
		errMsg := fmt.Errorf("cannot share files greater than %v bytes", maxUploadSizeLimitBytes)
		writeError(w, http.StatusBadRequest, apiCodeFileTooLarge, errMsg)
		return
	}

	msg.ReadOnly = true

	bs, _ = json.Marshal(msg)
	etag, err := getEtag(bs)
	if err != nil {
		writeError(w, http.StatusInternalServerError, apiCodeInternalError, err)
		return
	}

	msg.Etag = etag

	key := &StoreKey{Id: getUniqueID(), KeyType: KeyTypeLegacy}
	key, err = api.v1Store.Put(key, msg, api.getPrincipal(r))
	if err != nil {
		writeError(w, http.StatusInternalServerError, apiCodeInternalError, err)
		return
	}

	log.Debugf("Data uploaded successfully. Key: %v, Data: %+v", key, msg)

	oKey, err := key.toOpaque()
	if err != nil {
		writeError(w, http.StatusInternalServerError, apiCodeInternalError, fmt.Errorf("failed to create opaque key: %w", err))
		return
	}

	result := DataResponse{
		Result: fmt.Sprintf("%s/p/%s", strings.TrimSuffix(api.externalURL, "/"), oKey),
	}

	writeJSON(w, http.StatusOK, result)
}

// FIXME: The frontend should use the '/v2/publish/{key}' endpoint whenever possible to reuse existing gists also when on a new browser tab/window but for the user owning the original gist.
func (api *API) handlePublish(w http.ResponseWriter, r *http.Request) {
	api.doPublish(w, r, nil)
}

func (api *API) handleUpdatePublish(w http.ResponseWriter, r *http.Request) {
	api.doPublish(w, r, getKeyFromRequest(r))
}

// doPublish is a combined share/distribute handler (v2)
func (api *API) doPublish(w http.ResponseWriter, r *http.Request, key *StoreKey) {
	addCORSHeaders(w, r)

	if api.v2Store == nil {
		writeError(w, http.StatusInternalServerError, apiCodeInternalError, fmt.Errorf("v2 store not configured"))
		return
	}

	bs, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, apiCodeParseError, err)
		return
	}

	var msg DataRequest
	if err := util.UnmarshalJSON(bs, &msg); err != nil {
		writeError(w, http.StatusBadRequest, apiCodeParseError, err)
		return
	}

	if int64(len(bs)) > maxUploadSizeLimitBytes {
		errMsg := fmt.Errorf("cannot share files greater than %v bytes", maxUploadSizeLimitBytes)
		writeError(w, http.StatusBadRequest, apiCodeFileTooLarge, errMsg)
		return
	}

	msg.ReadOnly = true

	bs, _ = json.Marshal(msg)

	// This is a v2 call, only update the v2 store
	key, err = api.v2Store.Put(key, msg, api.getPrincipal(r))
	if err != nil {
		writeError(w, http.StatusInternalServerError, apiCodeInternalError, err)
		return
	}

	if etag := key.Revision; etag != "" {
		w.Header().Set("ETag", etag)
	}

	log.Debugf("Data uploaded successfully. Key: %v, Data: %+v", key, msg)

	commitKey, err := key.toOpaque()
	if err != nil {
		writeError(w, http.StatusInternalServerError, apiCodeInternalError, fmt.Errorf("failed to create opaque key: %w", err))
		return
	}

	bundleKey, err := key.withoutRevision().toOpaque()
	if err != nil {
		writeError(w, http.StatusInternalServerError, apiCodeInternalError, fmt.Errorf("failed to create opaque key: %w", err))
		return
	}

	result := DataResponse{
		BundleId:  bundleKey,
		BundleUrl: fmt.Sprintf("%s/p/%s", strings.TrimSuffix(api.externalURL, "/"), bundleKey),
		CommitId:  commitKey,
		CommitUrl: fmt.Sprintf("%s/p/%s", strings.TrimSuffix(api.externalURL, "/"), commitKey),
	}

	writeJSON(w, http.StatusOK, result)
}

func getGithubAuthToken(r *http.Request) (*oauth2.Token, error) {
	if authz := r.Header.Get("Authorization"); strings.HasPrefix(authz, "Bearer ") {
		token := strings.TrimPrefix(authz, "Bearer ")
		if token == "" {
			return nil, fmt.Errorf("invalid Authorization header")
		}
		return &oauth2.Token{AccessToken: token}, nil
	}

	cookie, err := r.Cookie(githubAuthCookie)
	if err != nil {
		return nil, err
	}

	decoded, err := base64.StdEncoding.DecodeString(cookie.Value)
	if err != nil {
		return nil, err
	}

	var cookieToken oauth2.Token
	if err := json.Unmarshal(decoded, &cookieToken); err != nil {
		return nil, err
	}

	return &cookieToken, nil
}

func (api *API) getPrincipal(r *http.Request) *Principal {
	token, err := getGithubAuthToken(r)
	if err != nil {
		return nil
	}

	return &Principal{
		oauthConfig: api.githubOauthConfig,
		accessToken: token,
	}
}

func (api *API) decodeKey(w http.ResponseWriter, r *http.Request) {
	s := getKeyFromRequest(r)

	baseURL, err := url.Parse("https://gist.github.com/")
	if err != nil {
		writeError(w, http.StatusInternalServerError, apiCodeInternalError, err)
		return
	}
	baseURL.Path = path.Join(baseURL.Path, s.Id, s.Revision)

	result := struct {
		URL string `json:"url"`
		ID  string `json:"id"`
		Rev string `json:"revision"`
	}{
		URL: baseURL.String(),
		ID:  s.Id,
		Rev: s.Revision,
	}

	writeJSON(w, http.StatusOK, result)
}

// testAuth won't redirect to the auth page, only checks if the user access token is valid or refreshes it
func (api *API) testAuth(w http.ResponseWriter, r *http.Request) {
	token, err := getGithubAuthToken(r)
	if err != nil {
		log.Debugf("error getting token: %v", err)
		writeError(w, http.StatusUnauthorized, apiCodeUnauthorized, errors.New("invalid token"))
		return
	}

	ctx := context.Background()
	// this will automatically refresh the token if it is expired
	latestToken, err := api.auth.Token(ctx, token)
	if err != nil {
		log.Debugf("error getting token: %v", err)
		writeError(w, http.StatusUnauthorized, apiCodeUnauthorized, errors.New("invalid token"))
		return
	}

	if !api.auth.Check(ctx, latestToken) {
		writeError(w, http.StatusUnauthorized, apiCodeUnauthorized, errors.New("invalid token"))
		return
	}

	b, err := json.Marshal(&latestToken)
	if err != nil {
		log.Debugf("error marshalling token: %v", err)
		writeError(w, http.StatusInternalServerError, apiCodeInternalError, errors.New("invalid token"))
		return
	}

	newCookie := http.Cookie{
		Name:     githubAuthCookie,
		Value:    base64.StdEncoding.EncodeToString(b),
		HttpOnly: true,
		Path:     "/",
	}
	http.SetCookie(w, &newCookie)

	writeJSON(w, http.StatusOK, "")
}

// handleGithubAuth redirects the user to get a new user access token
func (api *API) handleGithubAuth(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, api.githubOauthConfig.AuthCodeURL(""), http.StatusFound)
}

// handleGithubCallback is called by GitHub after user authorizes the GitHub OAuth app saving the user access token in a cookie.
func (api *API) handleGithubCallback(w http.ResponseWriter, r *http.Request) {
	callbackErr := r.URL.Query().Get("error")
	if callbackErr == "access_denied" {
		http.Redirect(w, r, fmt.Sprintf("%s#auth=denied", api.externalURL), http.StatusFound)
		return
	} else if callbackErr != "" {
		// TODO handle unknown errors more elegantly
		errDesc := r.URL.Query().Get("error_description")
		writeError(w, http.StatusInternalServerError, apiCodeInternalError, errors.New(callbackErr+":"+errDesc))
		return
	}

	code := r.URL.Query().Get("code")

	tok, err := api.auth.Exchange(context.Background(), code)
	if err != nil {
		writeError(w, http.StatusInternalServerError, apiCodeInternalError, err)
		return
	}

	b, err := json.Marshal(&tok)
	if err != nil {
		writeError(w, http.StatusInternalServerError, apiCodeInternalError, err)
		return
	}

	cookie := http.Cookie{
		Name:     githubAuthCookie,
		Value:    base64.StdEncoding.EncodeToString(b),
		HttpOnly: true,
		Path:     "/",
	}
	http.SetCookie(w, &cookie)

	http.Redirect(w, r, fmt.Sprintf("%s#auth=success", api.externalURL), http.StatusFound)
}

func getKeyFromRequest(r *http.Request) *StoreKey {
	params := mux.Vars(r)
	key, ok := params["key"]
	if !ok {
		return &StoreKey{Id: ""}
	}

	storeKey, err := storeKeyFromOpaque(key)
	if err != nil {
		return &StoreKey{Id: ""}
	}

	return storeKey
}

func (api *API) handleRetrieveInput(w http.ResponseWriter, r *http.Request) {
	key := getKeyFromRequest(r)

	log.Debugf("Trying to retrieve data for key %v\n", key)

	var msg DataRequest
	var found bool
	var err error

	if api.v2Store != nil && key.KeyType == KeyTypeGist {
		log.Debugf("Using v2 store for key %v", key)
		msg, found, err = api.v2Store.Get(key, api.getPrincipal(r))
	}

	if !found {
		log.Debugf("Using v1 store for key %v", key)
		// Fallback to v1 store
		msg, found, err = api.v1Store.Get(key, api.getPrincipal(r))
	}

	if err != nil {
		if errors.Is(err, &UnauthorizedError{}) {
			writeError(w, http.StatusUnauthorized, apiCodeUnauthorized, err)
			return
		}

		writeError(w, http.StatusInternalServerError, apiCodeInternalError, err)
		return
	}

	if !found {
		writeError(w, http.StatusNotFound, apiCodeNotFound, err)
		return
	}

	log.Debugf("Successfully Retrieved Data %+v for key %v", msg, key)

	result := InputResponse{
		Input: msg.Input,
	}

	writeJSON(w, http.StatusOK, result)
}

func (api *API) handleRetrieveBundle(w http.ResponseWriter, r *http.Request) {
	key := getKeyFromRequest(r)

	log.Debugf("Trying to retrieve data for key %v\n", key)

	etag := r.Header.Get("If-None-Match")

	var timeout time.Duration

	wait := getPreferHeaderField(r, "wait")
	if wait != "" {
		waitTime, err := strconv.Atoi(wait)
		if err != nil {
			writeError(w, http.StatusInternalServerError, apiCodeInternalError, err)
			return
		}
		timeout = time.Duration(waitTime) * time.Second
	}

	modes := []string{defaultBundleMode}
	modesVal := getPreferHeaderField(r, "modes")
	if modesVal != "" {
		modes = strings.Split(modesVal, ",")
	}

	api.doHandleRetrieveBundle(w, key, etag, timeout, modes, api.getPrincipal(r))
}

func (api *API) doHandleRetrieveBundle(w http.ResponseWriter, key *StoreKey, etag string, timeout time.Duration, modes []string, principal *Principal) {
	if timeout == 0 || etag == "" {
		api.doRegularPollMode(w, key, etag, modes, principal)
		return
	}

	ch := make(chan DataRequest)

	var found bool
	var err error

	if api.v2Store != nil && key.KeyType == KeyTypeGist {
		found, err = api.v2Store.Watch(key, etag, timeout, func(dr DataRequest) {
			ch <- dr
		}, principal)
	} else {
		found, err = api.v1Store.Watch(key, etag, timeout, func(dr DataRequest) {
			ch <- dr
		}, principal)
	}

	if err != nil {
		if errors.Is(err, &UnauthorizedError{}) {
			writeError(w, http.StatusUnauthorized, apiCodeUnauthorized, err)
		} else {
			writeError(w, http.StatusInternalServerError, apiCodeInternalError, err)
		}

		return
	}

	if !found {
		writeError(w, http.StatusNotFound, apiCodeNotFound, err)
		return
	}

	msg := <-ch

	if reflect.DeepEqual(msg, DataRequest{}) {
		writeError(w, http.StatusInternalServerError, apiCodeInternalError, fmt.Errorf("invalid data for key %v", key))
		return
	}

	if msg.Etag == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}

	if isDeltaBundleModeSupported(modes) && msg.Patch != nil {
		createAndWriteDeltaBundle(w, msg, key, etag)
	} else {
		createAndWriteSnapshotBundle(w, msg, key, etag)
	}
}

func (api *API) doRegularPollMode(w http.ResponseWriter, key *StoreKey, etag string, modes []string, principal *Principal) {
	var msg DataRequest
	var found bool
	var err error

	// First check the v2 store, then fall back to the v1 store
	if api.v2Store != nil && key.KeyType == KeyTypeGist {
		msg, found, err = api.v2Store.Get(key, principal)
	}

	if !found {
		msg, found, err = api.v1Store.Get(key, principal)
	}

	if err != nil {
		if errors.Is(err, &UnauthorizedError{}) {
			writeError(w, http.StatusUnauthorized, apiCodeUnauthorized, err)
		} else {
			writeError(w, http.StatusInternalServerError, apiCodeInternalError, err)
		}

		return
	}
	if !found {
		writeError(w, http.StatusNotFound, apiCodeNotFound, err)
		return
	}

	if msg.Etag == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}

	if etag != "" && isDeltaBundleModeSupported(modes) && msg.Patch != nil {
		createAndWriteDeltaBundle(w, msg, key, etag)
	} else {
		createAndWriteSnapshotBundle(w, msg, key, etag)
	}
}

func (api *API) handleRetrieveFromStore(w http.ResponseWriter, r *http.Request) {
	key := getKeyFromRequest(r)

	api.doHandleRetrieveFromStore(r.Context(), w, r.URL, key, api.getPrincipal(r))
}

func (api *API) doHandleRetrieveFromStore(ctx context.Context, w http.ResponseWriter, url *url.URL, key *StoreKey, principal *Principal) {
	log.Debugf("Trying to retrieve data for key %v\n", key)

	strict := getBoolParam(url, "strict", true)
	coverage := getBoolParam(url, "coverage", false)
	evaluate := getBoolParam(url, "evaluate", false)

	var msg DataRequest
	var found bool

	// First check the v2 store, then fall back to the v1 store
	if api.v2Store != nil && key.KeyType == KeyTypeGist {
		var err error
		msg, found, err = api.v2Store.Get(key, principal)
		if err != nil {
			var e *UnauthorizedError
			if errors.As(err, &e) {
				var code int
				var apiCode string
				if e.InvalidPrincipal {
					code = http.StatusUnauthorized
					apiCode = apiCodeUnauthorized
				} else {
					code = http.StatusForbidden
					apiCode = apiCodeForbidden
				}

				writeError(w, code, apiCode, e)
				return
			}
			var r *RateLimitedError
			if errors.As(err, &r) {
				writeError(w, http.StatusForbidden, apiCodeForbidden, r)
				return
			}

			writeError(w, http.StatusInternalServerError, apiCodeInternalError, err)
			return
		}
		if !found {
			writeError(w, http.StatusNotFound, apiCodeNotFound, err)
			return
		}
	}

	if !found {
		var err error
		msg, found, err = api.v1Store.Get(key, principal)
		if err != nil {
			writeError(w, http.StatusInternalServerError, apiCodeInternalError, err)
			return
		}
	}

	if !found {
		writeError(w, http.StatusNotFound, apiCodeNotFound, errors.New("key not found"))
		return
	}

	log.Debugf("Successfully Retrieved Data %+v for key %v", msg, key)

	keys := make([]string, len(msg.RegoModules))
	i := 0
	for k := range msg.RegoModules {
		keys[i] = k
		break
	}

	var policy string
	if len(keys) > 0 {
		policy = msg.RegoModules[keys[0]].(string)
	}

	response := DataResponse{
		Value:       policy,
		Input:       msg.Input,
		Data:        msg.Data,
		RegoVersion: msg.RegoVersion,
	}

	if coverage || evaluate {
		policies := make(map[string]string)
		for path, value := range msg.RegoModules {
			parts := strings.Split(path, "/")
			str, ok := value.(string)
			if !ok {
				writeError(w, http.StatusBadRequest, apiCodeParseError, errors.New("bad request"))
			}
			policies[parts[len(parts)-1]] = str
		}

		compileResult, ignored, err := opa.Compile(ctx, msg.Input, msg.Data, policies, msg.RegoQuery,
			msg.QueryPackage, msg.QueryImports, strict, msg.RegoVersion)
		if err != nil {
			log.WithError(err).Error("Compile Error.")
			writeErrorAndIgnored(w, http.StatusBadRequest, apiCodeParseError, err, ignored)
			return
		}

		result, evalErr := opa.Eval(
			ctx,
			compileResult,
			opa.EvalOptions{
				DebugTrace:          msg.Trace,
				Cover:               coverage,
				BuiltInErrorsAll:    msg.BuiltInErrorsAll,
				BuiltInErrorsStrict: msg.BuiltInErrorsStrict,
			},
		)
		if evalErr != nil {
			log.WithError(evalErr.RawError).Error("Eval Error.")
			writeErrorAndIgnored(w, evalErr.HTTPStatus, apiCodeInternalError, evalErr.RawError, ignored)
			return
		}

		if coverage {
			response.Coverage = result.Coverage
		}

		if evaluate {
			response.Result = result.Result
			response.EvalTime = result.Time
		}
	}

	writeJSON(w, http.StatusOK, response)
}

func (api *API) handleServeShared(w http.ResponseWriter, r *http.Request) {
	key := getKeyFromRequest(r)

	log.Debugf("Checking if key %v exists..\n", key)

	// Check if it exists... return a 404 if not, otherwise serve the UI

	var keys []*StoreKey

	// First check the v2 store, then fall back to the v1 store
	if api.v2Store != nil && key.KeyType == KeyTypeGist {
		var err error
		keys, err = api.v2Store.List(key, api.getPrincipal(r))
		if err != nil {
			expected := &UnauthorizedError{}
			if errors.As(err, &expected) {
				handler := http.StripPrefix(r.URL.Path, http.FileServer(http.Dir(api.contentRoot)))
				handler.ServeHTTP(w, r)
				return
			}

		}
	} else {
		var err error
		keys, err = api.v1Store.List(key, api.getPrincipal(r))
		if err != nil {
			writeError(w, http.StatusInternalServerError, apiCodeInternalError, err)
			return
		}
	}

	if len(keys) != 1 {
		writeError(w, http.StatusNotFound, apiCodeNotFound, errors.New("key not found"))
		return
	}

	log.Debugf("Successfully verified key %v", key)

	// Serve files with the prefix removed, basically the same as `/`
	handler := http.StripPrefix(r.URL.Path, http.FileServer(http.Dir(api.contentRoot)))
	handler.ServeHTTP(w, r)
}

func (api *API) handleVersion(w http.ResponseWriter, r *http.Request) {
	regalVersion := ""

	bi, ok := debug.ReadBuildInfo()
	if ok {
		for _, dep := range bi.Deps {
			if dep.Path == "github.com/styrainc/regal" {
				parts := strings.Split(dep.Version, "-")
				if len(parts) > 0 {
					regalVersion = parts[0]
				}
				break
			}
		}
	}

	// TODO: this has been added while Regal is not using a released version of OPA,
	// it can be reverted when Regal is using an unpatched OPA release again.
	presentedOPAVersion := version.OPAVersion
	parts := strings.Split(presentedOPAVersion, "-")
	if len(parts) > 1 {
		presentedOPAVersion = parts[0] + "+patch"
	}

	writeJSON(w, http.StatusOK, &VersionResponse{
		OPAVersion:        presentedOPAVersion,
		OPAReleaseVersion: version.OPAReleaseVersion,
		PlayGroundVersion: version.Version,
		PlayGroundVCS:     version.Vcs,
		RegalVersion:      regalVersion,
	})
}

func (api *API) handleVars(w http.ResponseWriter, r *http.Request) {
	bs, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, apiCodeParseError, err)
		return
	}

	var msg VarsRequest
	if err := util.UnmarshalJSON(bs, &msg); err != nil {
		writeError(w, http.StatusBadRequest, apiCodeParseError, err)
		return
	}

	if msg.RegoSelection == "" || msg.RegoModule == "" {
		writeError(w, http.StatusBadRequest, apiCodeInvalidArgument, errors.New("request must provide a module and selection"))
		return
	}

	result, opaErr := opa.VarsForSelection(msg.RegoModule, msg.RegoSelection)
	if opaErr != nil {
		writeError(w, opaErr.HTTPStatus, apiCodeParseError, opaErr.RawError)
		return
	}

	writeJSON(w, http.StatusOK, &VarsResponse{Result: result.Vars})
}

func (api *API) handleSession(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, "")
}

func (api *API) handleLiveness(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, "")
}

func (api *API) handleReadiness(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, "")
}

func (api *API) handleFormatting(w http.ResponseWriter, r *http.Request) {
	addCORSHeaders(w, r)

	bs, err := io.ReadAll(r.Body)
	var msg FormatRequest
	if err := util.UnmarshalJSON(bs, &msg); err != nil {
		writeError(w, http.StatusBadRequest, apiCodeParseError, err)
		return
	}

	if len(msg.RegoModule) == 0 {
		writeError(w, http.StatusBadRequest, apiCodeParseError, err)
		return
	}

	regoVersion := regoVersionFromRequest(msg.RegoVersion, ast.RegoUndefined)

	parseVersionAttempts := make([]ast.RegoVersion, 0)

	// if the user is using v0, attempt to format as v1 compatible first.
	if regoVersion == ast.RegoV0 || regoVersion == ast.RegoUndefined {
		parseVersionAttempts = append(parseVersionAttempts, ast.RegoV0CompatV1)
	}

	parseVersionAttempts = append(parseVersionAttempts, regoVersion)

	for _, v := range parseVersionAttempts {
		beginTimestamp := time.Now()
		var formatted []byte
		formatted, err = format.SourceWithOpts(
			"policy.rego",
			[]byte(msg.RegoModule),
			format.Opts{
				RegoVersion: v,
				// use the supplied version for parsing, but format as the attempt version
				ParserOptions: &ast.ParserOptions{RegoVersion: regoVersion},
			},
		)
		totalTime := time.Since(beginTimestamp)

		if err != nil {
			continue
		}

		responseVersion := 1
		if v == ast.RegoV0 {
			responseVersion = 0
		}

		writeJSON(w, http.StatusOK, &FormatResponse{
			Result:      string(formatted),
			RegoVersion: responseVersion,
			FormatTime:  totalTime.Nanoseconds(),
		})

		return
	}

	if err != nil {
		writeError(w, http.StatusBadRequest, apiCodeParseError, err)
		return
	}
}

func createAndWriteSnapshotBundle(w http.ResponseWriter, dr DataRequest, key *StoreKey, etag string) {
	files := make([]bundle.ModuleFile, 0, len(dr.RegoModules))

	for key, module := range dr.RegoModules {
		files = append(files, bundle.ModuleFile{
			Path: key,
			Raw:  []byte(module.(string)),
		})
	}

	var data map[string]interface{}
	var ok bool
	if dr.Data != nil {
		data, ok = (*(dr.Data)).(map[string]interface{})
		if !ok {
			writeError(w, http.StatusInternalServerError, apiCodeInternalError, errors.New("unable convert data to map[string]interface{}"))
			return
		}
	}

	b := bundle.Bundle{
		Manifest: bundle.Manifest{
			Revision: dr.Etag,
		},
		Modules: files,
		Data:    data,
	}

	w.Header().Set("ETag", dr.Etag)
	w.Header().Set("content-type", "application/vnd.openpolicyagent.bundles")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s.tar.gz", key.Id))
	w.WriteHeader(http.StatusOK)

	bundle.Write(w, b)
}

func createAndWriteDeltaBundle(w http.ResponseWriter, dr DataRequest, key *StoreKey, etag string) {
	patches := []bundle.PatchOperation{}

	for _, p := range *dr.Patch {
		var op string
		switch p.Operation {
		case "add":
			op = "upsert"
		case "remove", "replace":
			op = p.Operation
		default:
			writeError(w, http.StatusInternalServerError, apiCodeInternalError, fmt.Errorf("bad Patch operation: %v", p.Operation))
			return
		}

		patches = append(patches, bundle.PatchOperation{
			Op:    op,
			Path:  p.Path,
			Value: p.Value,
		})
	}

	b := bundle.Bundle{
		Manifest: bundle.Manifest{
			Revision: dr.Etag,
		},
		Patch: bundle.Patch{Data: patches},
	}

	w.Header().Set("ETag", dr.Etag)
	w.Header().Set("content-type", "application/vnd.openpolicyagent.bundles")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s.tar.gz", key.Id))
	w.WriteHeader(http.StatusOK)

	bundle.Write(w, b)
}

func writeError(w http.ResponseWriter, status int, code string, err error) {
	writeErrorAndIgnored(w, status, code, err, opa.Ignored{})
}

func writeErrorAndIgnored(w http.ResponseWriter, status int, code string, err error, ignored opa.Ignored) {
	var resp apiError
	resp.Code = code
	if err != nil {
		resp.Message = err.Error()
	}

	if isWritableError(err) {
		resp.Error = err
	}

	resp.Ignored = ignored

	writeJSON(w, status, resp)
}

func isWritableError(err error) bool {
	switch err.(type) {
	case *ast.Error,
		ast.Errors, // Could also be a slice of errors, mimick direct opa eval call by returning a JSON array in this case.
		rego.Errors,
		opa.WriteableBuiltInErrors,
		*topdown.Error:
		return true
	default:
		return false
	}
}

func writeJSON(w http.ResponseWriter, status int, x any) {
	bs, _ := json.Marshal(x)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	w.Write(bs)
}

func getUniqueID() string {
	b := make([]rune, 10)
	for i := range b {
		b[i] = letter[seededRand.Intn(len(letter))]
	}
	return string(b)
}

func getEtag(content []byte) (string, error) {
	hasher := sha256.New()
	w := bufio.NewWriterSize(hasher, 65536)

	if _, err := w.Write(content); err != nil {
		return "", err
	}

	if err := w.Flush(); err != nil {
		return "", err
	}

	digest := hasher.Sum(nil)
	hexDigest := make([]byte, hex.EncodedLen(len(digest)))
	hex.Encode(hexDigest, digest)
	return strconv.Quote(string(hexDigest)), nil
}

func getPreferHeaderField(r *http.Request, field string) string {
	for _, line := range r.Header.Values("prefer") {
		for _, part := range strings.Split(line, ";") {
			preference := strings.Split(strings.TrimSpace(part), "=")
			if len(preference) == 2 {
				if strings.ToLower(preference[0]) == field {
					return preference[1]
				}
			}
		}
	}
	return ""
}

func generateJSONDataPatch(original, new *DataRequest) error {
	var originalData map[string]interface{}
	var ok bool
	if original.Data != nil {
		originalData, ok = (*(original.Data)).(map[string]interface{})
		if !ok {
			return errors.New("unable convert data to map[string]interface{}")
		}
	}

	var newData map[string]interface{}
	if new.Data != nil {
		newData, ok = (*(new.Data)).(map[string]interface{})
		if !ok {
			return errors.New("unable convert data to map[string]interface{}")
		}
	}

	bsExisting, _ := json.Marshal(originalData)
	bsNew, _ := json.Marshal(newData)

	patch, err := jsonpatch.CreatePatch(bsExisting, bsNew)
	if err != nil {
		return errors.New("unable to create JSON data Patch")
	}

	if len(patch) != 0 {
		new.Patch = &patch
	}

	return nil
}

func isDeltaBundleModeSupported(modes []string) bool {
	for _, mode := range modes {
		if mode == deltaBundleMode {
			return true
		}
	}
	return false
}

// Add headers permitting any origin to execute calls against the playground.
// See https://www.w3.org/TR/cors/#resource-implementation and https://developer.mozilla.org/en-US/docs/Web/HTTP/CORS#Preflighted_requests for more info.
func addCORSHeaders(w http.ResponseWriter, r *http.Request) {
	// Add header to let the client know how long it can cache CORS responses.
	w.Header().Set("Access-Control-Max-Age", corsMaxAgeSec)

	// Add header to let the client know not to use a cached response if the origin or requested headers/method change.
	vary := w.Header().Get("Vary")
	if vary != "*" { // Only change it if not set to "*"
		if vary != "" { // Already set, add separator (https://developer.mozilla.org/en-US/docs/Web/HTTP/Headers/Vary)
			vary += ", "
		}
		vary += "Origin, Access-Control-Request-Headers, Access-Control-Request-Method"
		w.Header().Set("Vary", vary)
	}

	origin := r.Header.Get("Origin")

	// Add access control headers as applicable:
	w.Header().Set("Access-Control-Allow-Origin", origin)

	headers := r.Header.Get("Access-Control-Request-Headers")
	if headers != "" {
		w.Header().Set("Access-Control-Allow-Headers", headers)
	}

	methods := r.Header.Get("Access-Control-Request-Method") // (Requests single, !s)
	if methods != "" {
		w.Header().Set("Access-Control-Allow-Methods", methods) // (Allows multiple, s)
	}
}

type gzipHandler struct {
	handler http.Handler
}

type gzipResponseWriter struct {
	io.Writer
	http.ResponseWriter
}

func (w gzipResponseWriter) Write(b []byte) (int, error) {
	return w.Writer.Write(b)
}

func (gzh *gzipHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
		w.Header().Set("Content-Encoding", "gzip")
		gz := gzip.NewWriter(w)
		defer gz.Close()
		gzr := gzipResponseWriter{Writer: gz, ResponseWriter: w}
		gzh.handler.ServeHTTP(gzr, r)
	} else {
		gzh.handler.ServeHTTP(w, r)
	}
}

func getBoolParam(url *url.URL, name string, ifEmpty bool) bool {
	p, ok := url.Query()[name]
	if !ok {
		return ifEmpty
	}

	// Query params w/o values are represented as slice (of len 1) with an
	// empty string.
	if len(p) == 1 && p[0] == "" {
		return ifEmpty
	}

	for _, x := range p {
		if strings.ToLower(x) == "true" {
			return true
		}
	}

	return false
}

func regoVersionFromRequest(version *int, defaultVersion ast.RegoVersion) ast.RegoVersion {
	if version == nil {
		return defaultVersion
	}

	value := *version

	switch value {
	case 0:
		return ast.RegoV0
	case 1:
		return ast.RegoV1
	}

	return defaultVersion
}
