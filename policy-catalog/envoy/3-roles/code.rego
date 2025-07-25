# Roles
# -----
#
# This example grants admins the ability to do anything. This example shows
# how to extract claims from a JSON Web Token (JWT) and perform set lookups.
#
# For more information see:
#
# * Rego Policy Reference: https://www.openpolicyagent.org/docs/latest/policy-reference/

package envoy.http.roles

import rego.v1

default allow := false

# Sets are collections of values. To test if a value `x` is a member of `set`
# you can write `set[x]`, or `x in set`. In this case we check if the `"username"`
# claim is contained in the `admin_users` set.
allow if claims.username in admin_users

# admin_users is a set of usernames.
admin_users := {
	"alice",
	"bob",
}

# See the 'JWT Decoding' example for an explanation.
claims := payload if {
	v := input.attributes.request.http.headers.authorization
	startswith(v, "Bearer ")
	t := substring(v, count("Bearer "), -1)
	io.jwt.verify_hs256(t, "B41BD5F462719C6D6118E673A2389")
	[_, payload, _] := io.jwt.decode(t)
}
