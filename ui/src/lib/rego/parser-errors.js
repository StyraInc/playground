export class ParserSyntaxError extends SyntaxError {
  constructor(message, location, ...params) {
    super(message, ...params)

    if (Error.captureStackTrace) {
      Error.captureStackTrace(this, ParserSyntaxError)
    }

    this.name = 'ParserSyntaxError'
    this.location = location
  }
}

export class ParserKeywordError extends ParserSyntaxError {
  constructor(keyword, importKeyword, message, location, ...params) {
    super(message, location, ...params)

    this.name = 'ParserKeywordError'
    this.keyword = keyword
    this.import = importKeyword
  }
}

export class ParserTypeError extends TypeError {
  constructor(message, location, ...params) {
    super(message, ...params)

    if (Error.captureStackTrace) {
      Error.captureStackTrace(this, ParserTypeError)
    }

    this.name = 'ParserTypeError'
    this.location = location
  }
}
