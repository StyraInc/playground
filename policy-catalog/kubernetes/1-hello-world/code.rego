# Hello World
# -----------
#
# This example ensures that every resource that specifies a 'costcenter' label does
# so in the appropriate format. This example shows how to:
#
#	* Define rules that generate sets of error messages (e.g., `deny` below.)
#	* Refer to the data sent by Kubernetes in the Admission Review request.
#
# For additional information, see:
#
#	* Rego Rules: https://www.openpolicyagent.org/docs/latest/#rules
#	* Rego References: https://www.openpolicyagent.org/docs/latest/#references
#	* Kubernetes Admission Reviews: https://www.openpolicyagent.org/docs/latest/kubernetes-primer/#input-document

package kubernetes.validating.label

import rego.v1

# `deny` generates a set of error messages. The `msg` value is added to the set
# if the statements in the rule are true. If any of the statements are false or
# undefined, `msg` is not included in the set.
deny contains msg if {
	# `input` is a global variable bound to the data sent to OPA by Kubernetes. In Rego,
	# the `.` operator selects keys from objects. If a key is missing, no error
	# is generated. The statement is just undefined.
	value := input.request.object.metadata.labels.costcenter

	# Check if the label value is formatted correctly.
	not startswith(value, "cccode-")

	# Construct an error message to return to the user.
	msg := sprintf("Costcenter code must start with `cccode-`; found `%v`", [value])
}
