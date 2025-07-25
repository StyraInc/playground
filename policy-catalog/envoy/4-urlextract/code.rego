# URL Extraction
# --------------
#
# This example allows users to read their own profiles. This example shows how to:
#
# 	* Perform pattern matching on JSON values in Rego.
#	* Use Rego built-in functions to parse base64 encoded strings.
#	* Use parsed inputs provided by the OPA-Istio/Envoy integration.
#
# For more information see:
#
#	* Rego Built-in Functions: https://www.openpolicyagent.org/docs/latest/policy-reference/
#	* Equality: Assignment, Comparison, and Unification:
#     https://www.openpolicyagent.org/docs/latest/policy-language/#equality-assignment-comparison-and-unification
#	* OPA-Istio/Envoy Integration: https://github.com/open-policy-agent/opa-envoy-plugin

package envoy.http.urlextract

import rego.v1

default allow := false

allow if {
	input.attributes.request.http.method == "GET"

	# The `some` keyword declares local variables. This example declares a local
	# variable called `user_name` (used below).
	some user_name

	# The `=` operator in Rego performs pattern matching/unification. OPA finds
	# variable assignments that satisfy this expression (as well as all of the other
	# expressions in the same rule.)
	input.parsed_path = ["users", "profile", user_name]

	# Check if the `user_name` from path is the same as the username from the
	# credentials.
	user_name == basic_auth.user_name
}

basic_auth := {"user_name": user_name, "password": password} if {
	v := input.attributes.request.http.headers.authorization
	startswith(v, "Basic ")
	s := substring(v, count("Basic "), -1)
	[user_name, password] := split(base64url.decode(s), ":")
}
