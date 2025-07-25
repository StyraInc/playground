# Image Safety
# ------------
#
# This example prevents Kubernetes Pods from using containers from untrusted image
# registries. For simplicity, this example does NOT cover initContainers. To
# implement this policy, the rule needs to _search_ across the array of containers
# contained in every Pod resource. This example shows how to:
#
#	* Use the 'some' keyword to declare local variables.
#	* Iterate/search across JSON arrays.
#
# For additional information see:
#
#	* Rego `some` keyword: https://www.openpolicyagent.org/docs/latest/policy-language/#some-keyword
#	* Rego Iteration: https://www.openpolicyagent.org/docs/latest/#iteration

package kubernetes.validating.images

import rego.v1

deny contains msg if {
	input.request.kind.kind == "Pod"

	# The `some` keyword declares local variables. This rule declares a variable
	# called `container`, with the value any of the input request's spec's container
	# objects. It then checks if the container object's `"image"` field does not
	# start with "hooli.com/".
	some container in input.request.object.spec.containers
	not startswith(container.image, "hooli.com/")
	msg := sprintf("Image '%v' comes from untrusted registry", [container.image])
}
