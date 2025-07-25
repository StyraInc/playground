# Label Existence
# ---------------
#
# This example extends Hello World to ensure that all Kubernetes resources specify
# a 'costcenter' label and do so with the appropriate format. This example shows
# how to:
#
#	* Define policies with multiple rules that contribute to the same set (e.g., `deny` below.)
#	* Use the `not` keyword (a.k.a., negation) to test for undefined/missing fields.
#
# For additional information, see:
#
#	* Rego Logical OR: https://www.openpolicyagent.org/docs/latest/#logical-or
#	* Rego Negation: https://www.openpolicyagent.org/docs/latest/policy-language/#negation

package kubernetes.validating.existence

import rego.v1

# This definition checks if the costcenter label is not provided. Each rule definition
# contributes to the set of error messages.
deny contains msg if {
	# The `not` keyword turns an undefined statement into a true statement. If any
	# of the keys are missing, this statement will be true.
	not input.request.object.metadata.labels.costcenter
	msg := "Every resource must have a costcenter label"
}

# This definition checks if the costcenter label is formatted appropriately. Each rule
# definition contributes to the set of error messages.
deny contains msg if {
	value := input.request.object.metadata.labels.costcenter
	not startswith(value, "cccode-")
	msg := sprintf("Costcenter code must start with `cccode-`; found `%v`", [value])
}
