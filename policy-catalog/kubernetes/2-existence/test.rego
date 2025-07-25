package kubernetes.validating.test_existence

import rego.v1

import data.kubernetes.validating.existence as policy

test_validating_existence_positive_wrong_format if {
	actual := policy.deny with input as {"request": {"object": {"metadata": {"labels": {"costcenter": "foo"}}}}}

	# actual should be a set with 1 error message
	actual == {"Costcenter code must start with `cccode-`; found `foo`"}
}

test_validating_existence_positive_nonexistent if {
	actual := policy.deny with input as {"request": {"object": {"metadata": {}}}}

	# actual should be a set with 1 error message
	actual == {"Every resource must have a costcenter label"}
}

test_validating_existence_negative if {
	actual := policy.deny with input as {"request": {"object": {"metadata": {"labels": {"costcenter": "cccode-foo"}}}}}
	count(actual) == 0
}
