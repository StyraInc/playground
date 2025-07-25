// getGist fetches a gist revision and creates an object matching what is returned by /v1/data/{key}
async function getGist(gistID, sha) {
  try {
    const result = await fetch('https://api.github.com/gists/' + gistID + '/' + sha, {
      headers: {
        'Accept': 'application/vnd.github+json',
        'X-GitHub-Api-Version': '2022-11-28',
      }
    })

    const data = await result.json()
    const metadata = JSON.parse(data.files['rego_playground_metadata.json'].content)

    let jsonData = ""
    try {
      jsonData = JSON.parse(data.files['data.json'].content)
    } catch(e) {}
    let jsonInput = ""
    try {
      jsonInput = JSON.parse(data.files['input.json'].content)
    } catch(e) {}

    let content = {
      value: data.files['policy.rego'].content, // backend calls this `value` for some reason
      input: jsonInput,
      data: jsonData,
      coverage: metadata.coverage,
      rego_version: metadata.rego_version,
    }

    return content
  } catch (e) {
    return null
  }
}

// decodeKey gets the gist url
async function decodeKey(encoded) {
  const result = await fetch('/v2/decode/'+encoded)
  if (!result.ok) {
    throw Error(response.statusText)
  }
  return await result.json()
}

module.exports = {
  decodeKey: decodeKey,
  getGist: getGist
}