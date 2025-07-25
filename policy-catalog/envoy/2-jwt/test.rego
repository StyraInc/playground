package envoy.http.test_jwt

import rego.v1

import data.envoy.http.jwt as policy

test_http_jwt_positive if {
	inp := {"attributes": {"request": {"http": {
		"method": "POST",
		"path": "/pets/dogs",
		"headers": {"authorization": "Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJuYW1lIjoiQWxpY2lhIFNtaXRoc29uaWFuIiwicm9sZXMiOlsicmVhZGVyIiwid3JpdGVyIl0sInVzZXJuYW1lIjoiYWxpY2UifQ.md2KPJFH9OgBq-N0RonGdf5doGYRO_1miN8ugTSeTYc"}, # regal ignore:line-length
	}}}}

	policy.allow with input as inp
}

test_http_jwt_negative if {
	inp := {"attributes": {"request": {"http": {
		"method": "POST",
		"path": "/pets/cats",
		"headers": {"authorization": "Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJuYW1lIjoiQWxpY2lhIFNtaXRoc29uaWFuIiwicm9sZXMiOlsicmVhZGVyIiwid3JpdGVyIl0sInVzZXJuYW1lIjoiYWxpY2UifQ.md2KPJFH9OgBq-N0RonGdf5doGYRO_1miN8ugTSeTYc"}, # regal ignore:line-length
	}}}}

	not policy.allow with input as inp
}

test_token if {
	inp := {"attributes": {"request": {"http": {"headers": {"authorization": "Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJuYW1lIjoiQWxpY2lhIFNtaXRoc29uaWFuIiwicm9sZXMiOlsicmVhZGVyIiwid3JpdGVyIl0sInVzZXJuYW1lIjoiYWxpY2UifQ.md2KPJFH9OgBq-N0RonGdf5doGYRO_1miN8ugTSeTYc"}}}}} # regal ignore:line-length

	actual := policy.claims with input as inp
	actual == {
		"name": "Alicia Smithsonian",
		"roles": ["reader", "writer"],
		"username": "alice",
	}
}
