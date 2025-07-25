package envoy.http.test_urlextract

import rego.v1

import data.envoy.http.urlextract as policy

test_url_extract_positive if {
	policy.allow with input as {
		"attributes": {"request": {"http": {
			"method": "GET",
			"headers": {"authorization": "Basic Y2hhcmxpZTpwYXNzdzByZA=="},
		}}},
		"parsed_path": [
			"users",
			"profile",
			"charlie",
		],
	}
}

test_url_extract_negative if {
	not policy.allow with input as {
		"attributes": {"request": {"http": {
			"method": "GET",
			"headers": {"authorization": "Basic Y2hhcmxpZTpwYXNzdzByZA=="},
		}}},
		"parsed_path": [
			"users",
			"profile",
			"bob",
		],
	}
}
