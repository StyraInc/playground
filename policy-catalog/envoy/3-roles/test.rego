package envoy.http.test_roles

import rego.v1

import data.envoy.http.roles as policy

test_http_roles_positive if {
	inp := {"attributes": {"request": {"http": {
		"method": "POST",
		"path": "/pets/dogs",
		"headers": {"authorization": "Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJuYW1lIjoiQWxpY2lhIFNtaXRoc29uaWFuIiwicm9sZXMiOlsicmVhZGVyIiwid3JpdGVyIl0sInVzZXJuYW1lIjoiYWxpY2UifQ.md2KPJFH9OgBq-N0RonGdf5doGYRO_1miN8ugTSeTYc"}, # regal ignore:line-length
	}}}}

	policy.allow with input as inp
}

test_http_roles_negative if {
	inp := {"attributes": {"request": {"http": {
		"method": "DELETE",
		"path": "/pets/cats",
		"headers": {"authorization": "Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJ1c2VybmFtZSI6ImV2ZSIsIm5hbWUiOiJFbGVub3JlIFZpb2xhIEVkbW9uZHMiLCJyb2xlcyI6W119.QPLEIzrdz_0xss9udoWh5TgIRQWxN0R1bv6SFYxYWjY"}, # regal ignore:line-length
	}}}}

	not policy.allow with input as inp
}

test_token if {
	inp := {"attributes": {"request": {"http": {
		"method": "POST",
		"path": "/pets/dogs",
		"headers": {"authorization": "Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJuYW1lIjoiQWxpY2lhIFNtaXRoc29uaWFuIiwicm9sZXMiOlsicmVhZGVyIiwid3JpdGVyIl0sInVzZXJuYW1lIjoiYWxpY2UifQ.md2KPJFH9OgBq-N0RonGdf5doGYRO_1miN8ugTSeTYc"}, # regal ignore:line-length
	}}}}

	actual := policy.claims with input as inp
	actual == {
		"name": "Alicia Smithsonian",
		"roles": [
			"reader",
			"writer",
		],
		"username": "alice",
	}
}
