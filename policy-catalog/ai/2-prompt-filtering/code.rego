# AI: Prompt Filtering
# --------------------
# Generative AI tools which expose a chat-style interface to users typically
# need policy to ensure usage conforms with various organizational policies.
#
# This example system uses and endpoint, /v1/chat/completions, to accept lists
# of messages and generate responses. In example our request, the user has asked
# the following question 'Please tell me about user@example.com'.
#
# This example shows how we can control these submissions with the following
# policies:
# - total tokens in messages cannot exceed 100
# - messages from users cannot contain emails.
#
# Try removing the email address and making the system prompt shorter to see
# how the deny policy output responds.

package ai.chat

import rego.v1

# token limit will be enforced on all messages
token_limit := 100

deny contains message if {
	counts := [c |
		some msg in input.parsed_body.messages
		c := count(regex.split(`\s+`, msg.content))
	]

	total_tokens := sum(counts)
	total_tokens > token_limit

	message := sprintf("Total token count for messages cannot exceed %d (counted %d)", [token_limit, total_tokens])
}

deny contains message if {
	completions

	some i, emails in emails_in_user_messages

	message := sprintf("Messages cannot contain emails. Message %d contains: %v", [i + 1, concat(", ", emails)])
}

emails_in_user_messages := {i: emails} if {
	some i, msg in input.parsed_body.messages

	# only run the policy on messages in the history that are from the user
	msg.role == "user"

	emails := regex.find_n(`\S+@\S+`, msg.content, -1)

	count(emails) > 0
}

# completions will be true when handling a matching completions API request
completions if {
	input.parsed_path == ["v1", "chat", "completions"]
	input.attributes.request.http.method == "POST"
}
