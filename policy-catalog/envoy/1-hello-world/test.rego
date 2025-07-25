package envoy.http.test_public

import rego.v1

import data.envoy.http.public as policy

test_http_public_positive if {
	inp := {"attributes": {"request": {"http": {
		"method": "GET",
		"path": "/",
	}}}}

	policy.allow with input as inp
}

test_http_public_negative if {
	inp := {"attributes": {"request": {"http": {
		"method": "GET",
		"path": "/pets",
	}}}}

	not policy.allow with input as inp
}
