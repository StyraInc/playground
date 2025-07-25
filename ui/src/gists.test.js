const gist = require('./gists');
const fs = require('fs');
const path = require('path');

global.fetch = jest.fn(() =>
  Promise.resolve({
    json: () => {

      const filePath = path.join(__dirname, 'testdata/get_gist_response.json');
      const fileContent = fs.readFileSync(filePath, 'utf8');

      return Promise.resolve(JSON.parse(fileContent))
    }
  }),
);

test('read a gist', async() => {
  const data = await gist.getGist("123", "123")
  expect(JSON.stringify(data)).toBe(JSON.stringify(
    {
      value: 'I will become a gist!',
      input: '{"message":"world"}',
      data: '{}',
      coverage: false,
      rego_version: 1,
    }
  ))
});
