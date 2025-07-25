# AI: Model Access
# ----------------
# Generative AI tools which expose a chat-style interface to users typically
# need authorization rules to control access to different models. Some models
# may be more expensive to run, or contain sensitive data and should only be
# made available to a limited set of users.
#
# In this example we have a number of differed users identified by JSON Web Tokens.
# To run the example as a different user, update line 11 in the input on the right.
# You can change the model they're trying to access in line 21 of the input.
#
# Example Models
# model-1: our most basic model available to all users
# model-*-stage: the latest development releases, available to testers
# model-*-internal: model supplemented with internal data, only available to data-analysts
#
# Example Users
# alice: Data Analyst
#   token: eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIiwiaWF0IjoxNTE2MjM5MDIyLCJncm91cHMiOlsiZGF0YS1hbmFseXN0cyJdfQ.dpT70Zz1w0hmz8N4fjHcM8EwMPuJJt7VttqDt7UpLHw
# bob: Tester
#   token: eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIiwiaWF0IjoxNTE2MjM5MDIyLCJncm91cHMiOlsidGVzdGVycyJdfQ.2eoq7A3vj7KIAlrPZnHDS4VfAQIyfblkPImzTIk0PtA
# claire: Intern
#   token: eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIiwiaWF0IjoxNTE2MjM5MDIyLCJncm91cHMiOlsiaW50ZXJucyJdfQ.7Ozsiib3SYrp-nnLNncRygwpJcJ6ZnG4lu4BMvrYJqA
#
# Experiment with updates to the model field and the token to see how the deny policy responds.

package ai.chat

import rego.v1

# fail closed by default
default allow := false

# only allow when there are no denied violations in the request
allow if count(deny) == 0

deny contains message if {
	every pattern in all_accessible_models {
		not regex.match(pattern, input.parsed_body.model)
	}

	message := sprintf(
		"Model '%s' is not in your accessible models: %s",
		[input.parsed_body.model, concat(", ", all_accessible_models)],
	)
}

deny contains message if {
	not input.parsed_body.model
	message := "The 'model' key must be set in requests"
}

# model_access is a mapping of role to patterns which match models
# that users might be accessing.
model_access := {
	"interns": {"model-1"},
	"testers": {"model-1", `^model-\d+-stage$`},
	"data-analysts": {"model-1", `^model-\d+-internal$`},
}

all_accessible_models contains m if {
	some group in claims.groups
	some m in model_access[group]
}

claims := io.jwt.decode(bearer_token)[1] if {
	io.jwt.verify_hs256(bearer_token, "pa$$w0rd")
}

bearer_token := t if {
	v := input.attributes.request.http.headers.authorization
	startswith(v, "Bearer ")
	t := substring(v, count("Bearer "), -1)
}
