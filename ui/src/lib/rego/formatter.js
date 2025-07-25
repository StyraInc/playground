import * as Ast from './ast'

export default class Formatter {
  static format(ast, options) {
    const formatter = new Formatter()

    Formatter.format = function format(ast, options) {
      return formatter.format(ast, options)
    }

    return Formatter.format(ast, options)
  }

  constructor(options = {}) {
    this.reset = function (overrides = {}) {
      const {
        annotate = false,
        indent = '  ',
        inline = 0,
        level = 0,
        keywords = {},
        strict = true,
        trailer = '\n'
      } = {...options, ...overrides}

      this.annotate = annotate
      this.buffer = ''
      this.comment = ''
      this.comments = []
      this.indent = indent
      this.inline = inline
      this.isInline = inline > 0
      this.isWritingComments = false
      this.isWritingLine = false
      this.keywords = keywords
      this.level = level
      this.location = null
      this.nLines = 0
      this.strict = strict
      this.trailer = trailer
    }

    this.reset()
  }

  format(ast, options) {
    const keywords = {
      if: ast?.imports?.some(({path}) =>
        String(path) === 'future.keywords' || String(path) === 'future.keywords.if'
      ) ?? false
    }

    this.reset({keywords, ...options})

    try {
      this.comments = ast.comments || []
      this.write(new Array(this.level + 1).join(this.indent))
      this.writeTerm(ast)

      // TODO: Maybe don’t add trailing whitespace? See `writeComments()`.
      return this.buffer
        .replace(/[ \t]+$/gm, '') // Strip trailing whitespace.
        .replace(/(\S)\s*$/, `$1${this.trailer}`) // Add trailing characters.
    } finally {
      this.reset()
    }
  }

  // ---------------------------------------------------------------------------

  writeTerm(term) {
    if (term == null) {
      return
    }

    if (Array.isArray(term)) {
      const lastIndex = term.length - 1

      term.forEach((v, index) => {
        this.writeTerm(v)

        if (index < lastIndex) {
          this.write(', ')
        }
      })

      return
    }

    this.location = term.location || this.location
    this.writeComments(term.location)

    const type = term.constructor

    if (this.annotate) {
      this.write(`<${type.name}: `)
    }

    switch (type) {
      case Ast.Array: this.writeArray(term); break
      case Ast.ArrayComprehension: this.writeArrayComprehension(term); break
      case Ast.Boolean: this.write(`${term}`); break
      case Ast.Comment: this.write(`${term}`); break
      case Ast.Every: this.writeEvery(term); break
      case Ast.Expression: this.writeExpression(term); break
      case Ast.Import: this.writeImport(term); break
      case Ast.Null: this.write(`${term}`); break
      case Ast.Number: this.write(`${term}`); break
      case Ast.Object: this.writeObject(term); break
      case Ast.ObjectComprehension: this.writeObjectComprehension(term); break
      case Ast.Module: this.writeModule(term); break
      case Ast.Package: this.writePackage(term); break
      case Ast.Reference: this.writeReference(term); break
      case Ast.Rule: this.writeRule(term); break
      case Ast.RuleHead: this.writeHead(term); break
      case Ast.RuleBody: this.writeBody(term); break
      case Ast.Set: this.writeSet(term); break
      case Ast.SetComprehension: this.writeSetComprehension(term); break
      case Ast.String: this.write(JSON.stringify(`${term}`)); break
      case Ast.Some: this.writeSome(term); break
      case Ast.Variable: this.write(`${term}`); break
      case Ast.With: this.writeWith(term); break
      default: {
        let location = ''

        if (this.location) {
          const {location: {end: {line: end}, start: {line: start}}} = this
          location = ` at ${end > start ? `lines ${start} – ${end}` : `line ${start}`}`
        }

        throw Error(`Unexpected type for \`term\`${location}: ${JSON.stringify(term)}`)
      }
    }

    if (this.annotate) {
      this.write('>')
    }
  }

  writeComments(location) {
    if (location) {
      const before = []
      const comments = []
      let comment = this.comment

      this.comments.forEach((x) => {
        const diff = x.location.start.line - location.start.line

        if (diff < 0) {
          before.push(x)
        } else if (diff > 0) {
          comments.push(x)
        } else {
          comment = x
        }
      })

      this.comments = comments
      this.comment = comment

      this.startComments()

      const n = before.length

      before.forEach((comment, i) => {
        this.maybeWriteBlank(before[i - 1], comment)

        if (this.isWritingLine) {
          // XXX: We write the comment on the line started by the current term,
          // which means that we need to restart line indentation. When we next
          // call `this.maybeWriteBlank()`, it may end the line we started here,
          // resulting in trailing whitespace (i.e., the indentation).
          this.write(comment)
          this.endLine()
          this.startLine()
        } else {
          this.writeLine(comment)
        }
      })

      this.maybeWriteBlank(before[n - 1], {location})

      this.endComments()
    }
  }

  // ---------------------------------------------------------------------------

  writeModule(ast) {
    this.writePackage(ast.package)
    this.writeImports(ast.imports)
    this.writeRules(ast.rules)
  }

  writePackage(ast) {
    this.writeComments(ast.location)
    this.writeLine(`${ast}`)
    this.writeLine()
  }

  writeImports(imports) {
    if (imports.length > 0) {
      imports.forEach((ast) => {
        this.writeComments(ast.location)
        this.writeImport(ast)
      })

      this.writeLine()
    }
  }

  writeImport(ast) {
    this.writeLine(`${ast}`)
  }

  writeRules(rules) {
    rules.forEach((rule) => {
      this.writeComments(rule.location)
      this.writeRule(rule)
    })
  }

  writeRule(rule) {
    if (rule.isElse()) {
      this.writeElse(rule)
      return
    }

    const nLines = this.nLines

    if (rule.default) {
      this.write('default ')
    }

    this.writeHead(rule.head)

    const withIf = this.keywords.if
    const withBraces = !(withIf && rule.body.value.length === 1)
    this.writeBody(rule.body, withBraces, withIf)

    this.writeElse(rule.else)

    this.writeLine()

    if (this.nLines - nLines > 1) {
      this.writeLine()
    }
  }

  writeElse(rule) {
    if (rule == null) {
      return
    }

    // Add a space after the previous closing curly brace.
    //   ↓
    // '} '
    this.write(' ')

    const value = rule.head.value
    const isValueTrue = !!value && value.valueOf() === true
    const expressions = rule.body.value
    const isRuleTrue = isValueTrue && expressions.every((x) => x.isTrue())

    this.writeHead(rule.head)

    if (isRuleTrue) {
      this.write(' = true')
    } else {
      this.writeBody(rule.body)
    }

    this.writeElse(rule.else)
  }

  writeHead(head) {
    const {arguments: args, assign, key, name, value} = head
    const isElse = `${name}` === 'else'

    this.writeTerm(name)

    // `head.arguments` defines function inputs.
    if (!isElse && args) {
      const isInline = !this.isContextMultiline(head, this.strict)

      if (isInline) {
        this.startInline()
      }

      this.write('(')
      this.writeIterable(args, head)
      this.write(')')

      if (isInline) {
        this.endInline()
      }
    }

    // `head.key` defines a value in a partial set or a key in a partial object
    // key-value pair.
    if (key) {
      this.write('[')
      this.writeTerm(key)
      this.write(']')
    }

    // `head.value` defines function output, a complete rule value, or a value
    // in a partial object key-value pair.
    if (value && value.valueOf() !== true) {
      this.write(assign ? ' := ' : ' = ')
      this.writeTerm(value)
    }
  }

  writeBody(body, withBraces = true, withIf = false) {
    const {value: expressions, value: {length: n}} = body

    // If body is empty, leave it that way.
    if (body == null || expressions.every((x) => x.isTrue())) {
      return
    }

    const isInline = this.isInline || (withIf && !withBraces)
    const m = n - 1

    if (withIf) {
      this.write(` if${isInline ? ' ' : ''}`)
    }

    if (withBraces) {
      this.write(' {')
    }

    if (!isInline) {
      this.endLine()
      this.deeper()
    }

    for (let i = 0; i < n; ++i) {
      const expression = expressions[i]
      const nLines = this.nLines

      let addLine = 0

      if (!isInline) {
        addLine = this.maybeWriteBlank(expressions[i - 1], expression)
        this.startLine()
      }

      this.writeTerm(expression)

      if (isInline) {
        if (i < m) {
          this.write('; ')
        }
      } else {
        if (i < m) {
          this.endLine()

          if (this.nLines - nLines - addLine > 1) {
            this.writeLine()
          }
        }
      }
    }

    if (!isInline) {
      this.shallower()
      this.endLine()
      this.startLine()
    }

    if (withBraces) {
      this.write('}')
    }
  }

  // ---------------------------------------------------------------------------

  writeArray(array) {
    const {value: iterable, value: {length: n}} = array
    const isInline = this.isContextInline(array, iterable)

    this.write('[')

    if (n > 0) {
      if (!isInline) {
        this.endLine()
        this.deeper()
      }

      this.writeIterable(iterable, array)

      if (!isInline) {
        this.shallower()
        this.endLine()
        this.startLine()
      }
    }

    this.write(']')
  }

  writeArrayComprehension(comprehension) {
    const {value: {body, term}} = comprehension
    const isInline = this.isContextInline(comprehension)

    if (isInline) {
      this.startInline()
    }

    this.write('[')

    if (!this.isInline) {
      this.endLine()
      this.deeper()
      this.startLine()
      this.shallower()
    }

    this.writeTerm(term)
    this.write(' | ')
    this.writeBody(body, false)
    this.write(']')

    if (isInline) {
      this.endInline()
    }
  }

  writeCall(expression, parens = false) {
    const operator = expression.getOperator()
    const builtin = Ast.Builtin.get(operator)

    if (builtin && builtin.infix) {
      if (parens) {
        this.write('(')
      }

      this.writeTerm(expression.getLeftOperand())
      this.write(' ')
      this.write(builtin.infix)
      this.write(' ')
      this.writeTerm(expression.getRightOperand())

      if (parens) {
        this.write(')')
      }
    } else {
      const args = expression.getArguments()
      const isInline = !this.isContextMultiline(expression, this.strict)
      const m = args.length - 1

      this.write(operator)
      this.write('(')

      if (isInline) {
        this.startInline()

        args.forEach((term, i) => {
          this.writeTerm(term)

          if (i < m) {
            this.write(', ')
          }
        })

        this.endInline()
      } else {
        args.forEach((term, i) => {
          this.writeTerm(term)

          if (i < m) {
            this.write(', ')
          }
        })
      }

      this.write(')')
    }
  }

  writeEvery(every) {
    const [astExpression, astBody] = every.symbols
    this.writeExpression(astExpression)
    this.writeBody(astBody)
  }

  writeExpression(expression) {
    if (expression.negated) {
      this.write('not ')
    }

    if (expression.isCall()) {
      this.writeCall(expression)
    } else {
      this.writeTerm(expression.terms)
    }

    if (expression.with != null) {
      const n = expression.with.length
      const first = expression.with[0]
      const last = expression.with[n - 1]
      const isInline = !this.strict &&
        (first === last || last.location.start.line > first.location.start.line)

      if (!isInline) {
        this.endLine()
        this.deeper()
      }

      expression.with.forEach((x) => this.writeWith(x, isInline))

      if (!isInline) {
        this.shallower()
      }
    }
  }

  writeWith({target, value}, isInline = false) {
    if (isInline) {
      this.write(' with ')
    } else {
      this.startLine()
      this.write('with ')
    }

    this.writeTerm(target)
    this.write(' as ')
    this.writeTerm(value)
  }

  writeReference({value: segments}) {
    this.writeTerm(segments[0])

    for (let i = 1, ni = segments.length; i < ni; ++i) {
      const segment = segments[i]

      if (segment.constructor === Ast.String) {
        const string = Ast.Reference.isValidIdentifier(segment)
          ? `.${segment}`
          : `["${`${segment}`.replace(/"/g, '\\"')}"]`

        this.write(string)
      } else {
        this.write('[')
        this.writeTerm(segment)
        this.write(']')
      }
    }
  }

  writeObject(object) {
    const {value: entries, value: {length: n}} = object
    const isInline = this.isContextInline(object, entries)
    const m = n - 1

    this.write('{')

    if (!isInline) {
      this.endLine()
      this.deeper()
    }

    for (let i = 0; i < n; ++i) {
      const {[i]: [key, value]} = entries

      if (!isInline) {
        this.startLine()
      }

      this.writeTerm(key)
      this.write(': ')
      this.writeTerm(value)

      if (i < m) {
        if (isInline) {
          this.write(', ')
        } else {
          this.write(',')
          this.endLine()
        }
      }
    }

    if (!isInline) {
      this.shallower()
      this.endLine()
      this.startLine()
    }

    this.write('}')
  }

  writeObjectComprehension(comprehension) {
    const {value: {body, key, value}} = comprehension
    const isInline = this.isContextInline(comprehension)

    if (isInline) {
      this.startInline()
    }

    this.write('{')

    if (!this.isInline) {
      this.endLine()
      this.deeper()
      this.startLine()
      this.shallower()
    }

    this.writeTerm(key)
    this.write(': ')
    this.writeTerm(value)
    this.write(' | ')
    this.writeBody(body, false)
    this.write('}')

    if (isInline) {
      this.endInline()
    }
  }

  writeSet(set) {
    const {value: iterable, value: {size: n}} = set
    const isInline = this.isContextInline(set, iterable)

    if (n === 0) {
      this.write('set()')
    } else {
      this.write('{')

      if (!isInline) {
        this.endLine()
        this.deeper()
      }

      this.writeIterable(iterable, set)

      if (!isInline) {
        this.shallower()
        this.endLine()
        this.startLine()
      }

      this.write('}')
    }
  }

  writeSetComprehension(comprehension) {
    const {value: {body, term}} = comprehension
    const isInline = this.isContextInline(comprehension)

    if (isInline) {
      this.startInline()
    }

    this.write('{')

    if (!this.isInline) {
      this.endLine()
      this.deeper()
      this.startLine()
      this.shallower()
    }

    this.writeTerm(term)
    this.write(' | ')
    this.writeBody(body, false)
    this.write('}')

    if (isInline) {
      this.endInline()
    }
  }

  writeIterable(iterable, context) {
    const isInline = this.isContextInline(context, iterable)
    const n = iterable[typeof iterable.size === 'number' ? 'size' : 'length']
    const m = n - 1

    let i = 0

    if (isInline) {
      iterable.forEach((term) => {
        this.writeTerm(term)

        if (i < m) {
          this.write(', ')
        }

        ++i
      })
    } else {
      iterable.forEach((term) => {
        this.startLine()
        this.writeTerm(term)

        if (i < m) {
          this.write(',')
          this.endLine()
        }

        ++i
      })
    }
  }

  writeSome(some) {
    const {symbols, symbols: {length}} = some
    const n = length - 1

    if (Array.isArray(symbols)) {
      symbols.forEach((v, i) => this.write(i === n ? v : `${v}, `))
    } else {
      this.writeTerm(symbols)
    }
  }

  // ---------------------------------------------------------------------------

  isContextInline(context, iterable = {size: 2}) {
    const n = iterable[('size' in iterable) ? 'size' : 'length']

    if (this.strict) {
      return this.isInline || n < 1
    }

    const {location} = context

    let end = 0
    let start = 0

    if (location) {
      end = location.end.line
      start = location.start.line
    }

    return this.isInline || n < 1 || end - start === 0
  }

  isContextMultiline(context, isStrictlyInline = false) {
    if (isStrictlyInline && this.strict) {
      return false
    }

    const {location} = context

    let end = 0
    let start = 0

    if (location) {
      end = location.end.line
      start = location.start.line
    }

    return end - start > 0
  }

  // ---------------------------------------------------------------------------

  /**
   * Writes a single blank line between `prevPart` and `nextPart` if there was
   * originally one or more lines between them.
   *
   * @param {Ast.Term} prevPart - The first block of code in a series.
   * @param {Ast.Term} nextPart - The second block of code in a series.
   *
   * @return {number} The number of lines (`0` or `1`) written.
   */
  maybeWriteBlank(prevPart, nextPart) {
    const next = nextPart ? nextPart.location.start.line : 0
    const prev = prevPart ? prevPart.location.end.line : next
    const diff = next - prev

    // TODO: We use a regexp to look behind at `this.buffer` to determine
    // whether there already exist adequate blank lines. Can we avoid this?
    const nLines = diff < 2 || /\n{2,}$/.test(this.buffer) ? 0 : 1

    if (nLines) {
      if (this.isWritingLine) {
        this.endLine()
        this.startLine()
      } else {
        this.writeLine()
      }
    }

    return nLines
  }

  write(string = '') {
    this.buffer += string
  }

  writeLine(string) {
    if (string) {
      this.startLine()
      this.write(string)
    }

    this.endLine()
  }

  // ---------------------------------------------------------------------------

  deeper() {
    this.level++
  }

  shallower() {
    this.level--
  }

  startComments() {
    this.isWritingComments = true
  }

  endComments() {
    this.isWritingComments = false
  }

  startInline() {
    this.inline++
    this.isInline = this.inline > 0
  }

  endInline() {
    this.inline--
    this.isInline = this.inline > 0
  }

  startLine() {
    if (this.isWritingLine) {
      this.endLine()
    }

    this.isWritingLine = true
    this.write(new Array(this.level + 1).join(this.indent))
  }

  endLine() {
    this.isWritingLine = false

    if (!this.isWritingComments) {
      this.nLines++

      if (this.comment) {
        this.write(` ${this.comment}`)
        this.comment = ''
      }
    }

    if (this.isInline) {
      this.write(' ')
    } else {
      this.write('\n')
    }
  }
}

export const {format} = Formatter
