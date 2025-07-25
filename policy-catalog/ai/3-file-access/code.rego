# AI: Files
# ---------
# Controlling access to API endpoints used for model customization is an important
# policy for any AI sytem. In this example, we show how to ensure that only users
# with in the 'trainer' group can upload files to our system.

# This example system uses /v1/files to accept file uploads for training purposes
# and we'll be guarding access to this endpoint.

# Example token with trainer group for testing:
# eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIiwibmFtZSI6IkphbmUgRG9lIiwiaWF0IjoxNTE2MjM5MDIyLCJncm91cHMiOlsidHJhaW5lciJdfQ.UaacvR8F-4Sc3ZnV7EOBP9dl2f-7Gfn8e-uymDKJYeY
# Update the 'authorization' header in the input to 'Bearer ...' and inspect
# the change in the output of the deny rule.

package ai.files

import rego.v1

deny contains message if {
	post
	not "trainer" in claims.groups
	message := "Group 'trainer' is required to access this endpoint"
}

post if {
	input.parsed_path == ["v1", "files"]
	input.attributes.request.http.method == "POST"
}

claims := io.jwt.decode(bearer_token)[1] if {
	io.jwt.verify_hs256(bearer_token, "pa$$w0rd")
}

bearer_token := t if {
	v := input.attributes.request.http.headers.authorization
	startswith(v, "Bearer ")
	t := substring(v, count("Bearer "), -1)
}
