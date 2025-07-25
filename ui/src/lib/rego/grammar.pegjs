{
  const CHECK_FUTURE_KEYWORDS = Symbol.for('checkFutureKeywords')

  const KEYWORDS = Ast.KEYWORDS.reduce((keywords, keyword) => {
    keywords[keyword] = true
    return keywords
  }, {[CHECK_FUTURE_KEYWORDS]: true})

  const parseErrors = []

  // future keywords requires opt-in `import.future.keywords.<keyword>`
  Ast.FUTURE_KEYWORDS.forEach((keyword) => {
    KEYWORDS[keyword] = false  // true once opted-in
  })

  const astModule = new Ast.Module()
  function addStatement(statement) {
    assertPackage(statement)
    astModule.addStatement(statement)
    return statement
  }

  function annotate(message, location) {
    const {line, column} = location.start
    return `${message} at line ${line}, column ${column}`
  }

  function optInFutureKeyword(path, alias, location) {
    const type = path?.constructor
    const values = path?.value

    if (!type === Ast.Reference || !values) {
      return
    }

    if (values?.length < 2) {
      return
    }

    const isFutureKeywords = String(values[0]) === Ast.Variable.FUTURE_ROOT.value
      && String(values[1]) === 'keywords'

    if (!isFutureKeywords) {
      return
    }

    if (alias) {
      const message = 'Future keyword imports cannot be aliased'
      throw new Errors.ParserTypeError(message, location)
    }

    const keyword = String(values?.[2] ?? '')
    if (keyword) {
      // opt-in keyword
      KEYWORDS[keyword] = true
    } else {
      // opt-in all keywords
      Ast.FUTURE_KEYWORDS.forEach((keyword) => {
        KEYWORDS[keyword] = true
      })
    }
  }

  let hasPackage = false
  function assertPackage(statement) {
    // XXX: Using `peg$startRuleFunction` and `peg$parseModule` from generated
    // parser internals as a way to know whether to expect a package definition.
    if (peg$startRuleFunction === peg$parseModule) {
      const type = statement && statement.constructor

      if (hasPackage) {
        if (type === Ast.Package) {
          const message = `Unexpected ${Ast.Package.TYPE}`
          const {location} = statement
          throw new Errors.ParserSyntaxError(message, location)
        }
      } else if (type === Ast.Package) {
        hasPackage = true
      } else if (type !== Ast.Comment) {
        const message = `Expected ${Ast.Package.TYPE}`
        const {location} = statement
        throw new Errors.ParserSyntaxError(message, location)
      }
    }
  }

  const rootValues = new Set([Ast.Variable.DEFAULT_ROOT.value,
    Ast.Variable.INPUT_ROOT.value, Ast.Variable.FUTURE_ROOT.value])
  function assertValidImportPath(path, location) {
    const type = path?.constructor
    const value = path?.value

    let message
    if (type === Ast.Variable) {
      if (!rootValues.has(value)) {
        message = `Path must begin with \`input\` or \`data\` or \`future\``
      }
    } else if (type === Ast.Reference) {
      assertValidImportPath(value[0], location)
      for (let i = 1, ni = value.length; i < ni; ++i) {
        if (value[i].constructor !== Ast.String) {
          message = `Path elements must be ${Ast.String.TYPE} types`
        }
      }
    } else {
      // Technically not possible if `Import` rule is defined correctly. We will
      // get a `ParseError` instead.
      message = `Path must be ${Ast.Reference.TYPE} or ${Ast.Variable.TYPE} type`
    }


    if (message) {
      throw new Errors.ParserTypeError(`Invalid path ${String(path)}: ${message}`, location)
    }
  }

  function assertValidImportAlias(alias, location) {
    const aliasStr = String(alias)

    if (aliasStr === 'data' || aliasStr === 'input') {
      const message = 'Alias cannot be \`input\` or \`data\`'
      throw new Errors.ParserTypeError(`Invalid alias ${aliasStr}: ${message}`, location)
    }
  }

  const commentsByLocation = {}
  function deduplicateComment(comment) {
    // XXX: The parser duplicates comments. Ignore them.
    const location = JSON.stringify(comment.location)
    if (!commentsByLocation[location]) {
      commentsByLocation[location] = true
      return addStatement(comment)
    }
  }

  function makeOperator() {
    const name = new Ast.Variable(Ast.Builtin.get(text()).name, location())
    return new Ast.Reference(name, location())
  }

  function makeExpression(head, rest) {
    if (rest.length === 0) {
      return head
    }

    const operator = rest[0][1]
    const tail = rest[0][3]
    return new Ast.Expression([operator, head, tail], location())
  }

  function makeRuleFromExpression(expression) {
    if (expression.negated) {
      const type = expression.constructor.TYPE
      const message = `Negated ${type} cannot be used for ${Ast.Rule.TYPE}`
      const {location} = expression
      throw new Errors.ParserTypeError(message, location)
    }

    if (expression.with) {
      const type = `Expression using \`${Ast.With.TYPE}\``
      const message = `${type} cannot be used as ${Ast.RuleHead.TYPE}`
      const {location} = expression
      throw new Errors.ParserTypeError(message, location)
    }

    if (!expression.isCall()) {
      return makePartialSetRuleFromTerm(expression)
    }

    if (!expression.isAssignment() && !expression.isEquality()) {
      return makeRuleFromCall(expression)
    }

    const leftOperand = expression.getLeftOperand()
    const rightOperand = expression.getRightOperand()

    const rule = makeObjectRuleFromEquality(leftOperand, rightOperand)

    if (expression.isAssignment()) {
      rule.head.assign = true
    }

    return rule
  }

  function makeRuleFromCall(expression) {
    const location = expression.location

    const name = expression.getOperator()
    if (Ast.Builtin.get(name)) {
      if (name === 'assign') {
        makeCompleteObjectRuleFromAssignment
      }

      const message = `Built-in function cannot be incrementally defined`
      throw new Errors.ParserTypeError(message, location)
    }

    const args = expression.getInputs()
    if (!args) {
      const message = `Expected ${Ast.Rule.TYPE} with at least one argument`
      throw new Errors.ParserSyntaxError(message, location)
    }

    const value = expression.getOutput() ?? new Ast.Boolean(true, location)
    const head = new Ast.RuleHead(name, args, value, location)
    const body = Ast.RuleBody.makeTrue(location)

    return new Ast.Rule(head, body, location)
  }

  function makeObjectRuleFromEquality(leftOperand, rightOperand) {
    return makeCompleteObjectRuleFromEquality(leftOperand, rightOperand) ??
      makePartialObjectRuleFromEquality(leftOperand, rightOperand)
  }

  function makeCompleteObjectRuleFromEquality(leftOperand, rightOperand) {
    const type = leftOperand && leftOperand.constructor
    if (type !== Ast.Expression && type !== Ast.Reference && type !== Ast.Variable) {
      return // Fall through to `makePartialObjectRuleFromEquality()`.
    }

    let name = leftOperand
    let argumentsOrKey = null

    if (type === Ast.Expression && leftOperand.isCall()) {
      name = leftOperand.terms[0]
      argumentsOrKey = leftOperand.terms.slice(1)
    }

    if (type === Ast.Reference) {
      name = leftOperand.value[0]

      const defaultRoot = Ast.Variable.DEFAULT_ROOT.value
      const inputRoot = Ast.Variable.INPUT_ROOT.value
      const nameValue = name.value

      if (nameValue !== defaultRoot && nameValue !== inputRoot) {
        return
      }
    }

    const location = {
      end: rightOperand.location.end,
      start: leftOperand.location.start
    }

    const value = rightOperand
    const head = new Ast.RuleHead(name, argumentsOrKey, value, location)
    const body = Ast.RuleBody.makeTrue(location)

    return new Ast.Rule(head, body, location)
  }

  function makePartialObjectRuleFromEquality(leftOperand, rightOperand) {
    if (!(leftOperand instanceof Ast.Reference)) {
      const message = `Type \`${type.TYPE}\` cannot be used for name of ${Ast.Rule.TYPE}`
      const {location} = leftOperand
      throw new Errors.ParserTypeError(message, location)
    }

    const location = {
      end: rightOperand.location.end,
      start: leftOperand.location.start
    }

    // create reference for name if segments length > 2
    const key = leftOperand.value.pop()
    const name = leftOperand.value.length > 1
      ? new Ast.Reference(leftOperand.value, {
          start: leftOperand.location.start,
          end: leftOperand.value.at(-1).location.end
        })
      : leftOperand.value[0]
    
    const value = rightOperand

    const head = new Ast.RuleHead(name, key, value, location)
    const body = Ast.RuleBody.makeTrue(location)

    return new Ast.Rule(head, body, location)
  }

  function makePartialSetRuleFromTerm(expression) {
    const loc = expression.location
    const reference = expression.terms
    const termsType = reference && reference.constructor

    if (termsType !== Ast.Reference) {
      const message = `Type \`${termsType.TYPE}\` cannot be used for ${Ast.RuleHead.TYPE}: ${expression}`
      throw new Errors.ParserTypeError(message, loc)
    }

    const root = reference.value[0]

    if (root.constructor !== Ast.Variable) {
      const message = `Type \`${root.constructor.TYPE}\` cannot be used for ${Ast.Rule.TYPE}`
      throw new Errors.ParserTypeError(message, loc)
    }

    let head
    if (reference.value.length === 2) {
      const name = root
      const key = reference.value[1]
      const value = reference.value[2]

      head = new Ast.RuleHead(name, key, value, loc)
    } else {
      head = new Ast.RuleHead(reference, null, new Ast.Boolean(true, location()), loc) 
    }
    
    const body = Ast.RuleBody.makeTrue(loc)
    return new Ast.Rule(head, body, loc)
  }

  function makeElseRule(head, extension) {
    const {body, location, term} = extension

    // Generate a location for the `else` keyword. Reuse the start of the
    // extension; offset the end 4 characters (the length of 'else') from
    // the start.
    const nameLocation = Ast.makeLocation(location)
    nameLocation.end.column = nameLocation.start.column + 4
    nameLocation.end.line = nameLocation.start.line
    nameLocation.end.offset = nameLocation.start.offset + 4

    const name = new Ast.Variable('else', nameLocation)
    const argumentsOrKey = head.arguments ?? head.key
    const elseHead = new Ast.RuleHead(name, argumentsOrKey, term, term.location)

    return new Ast.Rule(elseHead, body, location)
  }

  function mangleDefaultRoot(variable, location) {
    if (variable && variable.value === Ast.Variable.DEFAULT_ROOT.value) {
      return new Ast.Reference(variable, location)
    }

    return variable
  }

  function mangleInputRoot(variable, location) {
    if (variable && variable.value === Ast.Variable.INPUT_ROOT.value) {
      return new Ast.Reference(variable, location)
    }

    return variable
  }

  const wildcard = '_'
  const wildcardPrefix = '$'
  const wildcardsByLocation = {}

  let nWildcards = 0

  function mangleWildcard(variable) {
    if (variable && variable.value === wildcard) {
      const location = JSON.stringify(variable.location)
      const wildcard = wildcardsByLocation[location]

      // XXX: The parser duplicates variables. Ignore them.
      if (wildcard) {
        return wildcard
      }

      variable.value = `${wildcardPrefix}${nWildcards}`
      nWildcards++

      wildcardsByLocation[location] = variable
    }

    return variable
  }

  function mangleVariable(variable, location) {
    return mangleWildcard(mangleInputRoot(mangleDefaultRoot(variable, location), location))
  }
}

// -----------------------------------------------------------------------------

Module = _ Statement Statements* _ EOF {
  return astModule
}

Statement = Package / Import / Rule / Comment / ParseError

Statements = (ws / ParseError) statement:Statement {
  parseErrors.length = 0
  return statement
}

// -----------------------------------------------------------------------------

Array = '[' _ head:RecursiveExpression? tail:RecursiveExpressions* _ ','? _ ']' {
  if (!head) {
    return new Ast.Array(undefined, location())
  }

  return new Ast.Array([head].concat(tail), location())
}

ArrayComprehension = '[' _ term:Term _ '|' _ body:BlockBody _ ']' {
  return new Ast.ArrayComprehension(term, body, location())
}

// -----------------------------------------------------------------------------

BlockBody = head:Expression tail:BlockBodyExpressions* {
  return new Ast.RuleBody([head].concat(tail), location()).index()
}

BlockBodyBreaks = [ \t]* ((';' Comment?) / (Comment? [\r\n])) _

BlockBodyExpressions = BlockBodyBreaks expression:Expression {
  return expression
}

// -----------------------------------------------------------------------------

Call = name:CallName _ args:ArgumentList {
  if (name.value[0].value === 'set') {
    if (args == null) {
      return new Ast.Set(undefined, location())
    }

    throw new Errors.ParserTypeError(`Invalid empty set constructor \`${text()}\``, location())
  }

  return new Ast.Expression([name].concat(args), location())
}

// when a keyword has the same name as a builtin function, disable the keyword check
CallName = UncheckFutureKeyword name:(Reference / Variable) {
  KEYWORDS[CHECK_FUTURE_KEYWORDS] = true
  return Ast.isReference(name) ? name : new Ast.Reference(name, location())
}

UncheckFutureKeyword = &{
  KEYWORDS[CHECK_FUTURE_KEYWORDS] = false
  return true
}

// -----------------------------------------------------------------------------

Comment = [ \t]* '#' value:([^\r\n])* {
  return deduplicateComment(new Ast.Comment(value.join(''), location()))
}

// -----------------------------------------------------------------------------

Expression = Some / TermExpression / Term

ExpressionOnly = _ expression:(Expression) _ {
  return expression
}

Every = 'every' ws symbols:EverySymbols {
  if (!KEYWORDS['every']) {
    const importKeyword = 'future.keywords.every'
    const message = `\`import ${importKeyword}\` opt-in to use \`every\` keyword`
    throw new Errors.ParserKeywordError('every', importKeyword, message, location())
  }

  const loc = location()
  return new Ast.Expression(new Ast.Every(symbols, loc), loc)
}

EverySymbols = EveryVariables

EveryVariables = head:InRecursiveExpression ws tail:RuleBody {
  return [head].concat(tail)
}

Some = 'some' ws symbols:SomeSymbols {
  if (!symbols) {
    const message = `Expected symbols \`${text()}\``
    throw new Errors.ParserSyntaxError(message, location())
  }

  const loc = location()
  return new Ast.Expression(new Ast.Some(symbols, loc), loc)
}

SomeSymbols = InRecursiveExpression / SomeVariables

SomeVariables = head:Variable tail:Variables* {
  return [head].concat(tail)
}

TermExpression = negation:('not' ws)? expression:AnyExpression withs:With* {
  expression.setNegated(!!negation)

  // Fix up `expression` location to include the `not` keyword (if any) as well
  // as any intervening whitespace, comments, or both.
  expression.location.start = location().start

  withs.forEach((nextWith) => expression.addWith(nextWith))
  return expression
}

HeadAnyExpression = Every / RecursiveExpression

AnyExpression = head:(HeadAnyExpression) rest:(_ AssignOperator _ RecursiveExpression)? {
  if (!rest) {
    const type = head.constructor
    const terms = type === Ast.Expression ? head.terms : head
    return new Ast.Expression(terms, location())
  }

  const operator = rest[1]
  const tail = rest[3]
  return new Ast.Expression([operator, head, tail], location())
}

AssignOperator = (':=' / '=') {
  return makeOperator()
}

RecursiveExpressions = _ ',' _ expression:RecursiveExpression {
  return expression
}

RecursiveExpression = InRecursiveExpression / BuiltinRecursiveExpression

InRecursiveExpression = head:InRecursiveQuery ws operator:InOperator ws rest:(RecursiveExpression)+ {
  if (!KEYWORDS['in'] && !KEYWORDS['every']) {
    const importKeyword = 'future.keywords.in'
    const message = `\`import ${importKeyword}\` opt-in to use \`in\` keyword`
    throw new Errors.ParserKeywordError('in', importKeyword, message, location())
  }

  return new Ast.Expression([operator, head, rest], location())
}

InRecursiveQuery = head:(BuiltinRecursiveExpression) tail:(_ ',' _ BuiltinRecursiveExpression)? {
  return tail ? [head, tail[3]] : [head]
}

InOperator = 'in' {
  const name = new Ast.Variable('in', location())
  return new Ast.Reference(name, location())
}

BuiltinRecursiveExpression = head:Comparison rest:(_ ComparisonOperator _ Comparison)* {
  return makeExpression(head, rest)
}

ComparisonOperator = ('==' / '!=' / '<=' / '>=' / '<' / '>') {
  return makeOperator()
}

Comparison = head:BitwiseOr rest:(_ BitwiseOrOperator _ BitwiseOr)* {
  return makeExpression(head, rest)
}

BitwiseOrOperator = '|' {
  return makeOperator()
}

BitwiseOr = head:BitwiseAnd rest:(_ BitwiseAndOperator _ BitwiseAnd)* {
  return makeExpression(head, rest)
}

BitwiseAndOperator = '&' {
  return makeOperator()
}

BitwiseAnd = head:AddSubtract rest:(_ AddSubtractOperator _ AddSubtract)* {
  return makeExpression(head, rest)
}

AddSubtractOperator = ('+' / '-') {
  return makeOperator()
}

AddSubtract = head:MultiplyDivide rest:(_ MultiplyDivideOperator _ MultiplyDivide)* {
  return makeExpression(head, rest)
}

MultiplyDivideOperator = ('*' / '/' / '%') {
  return makeOperator()
}

MultiplyDivide = Recurse / BaseCase

Recurse = '(' _ head:RecursiveExpression _ ')' tail:(References)* {
  if (tail.length > 0) {
    return new Ast.Reference([head].concat(tail), location())
  }

  return head
}

BaseCase = term:Term {
  return term
}

ComparisonAssignment = _ operator:(ComparisonOperator / AssignOperator) _ {
  return operator
}

With = ws loc:WithLocation ws target:(Reference / Variable) ws 'as' ws value:Term {
  return new Ast.With(target, value, loc)
}

WithLocation = 'with' !IdentifierTail {
  return location()
}

// -----------------------------------------------------------------------------

False = 'false' !IdentifierTail {
  return new Ast.Boolean(false, location())
}

// -----------------------------------------------------------------------------

Import = 'import' ws path:(Reference / Variable) alias:Alias? {
  const loc = location()

  assertValidImportPath(path, loc)
  assertValidImportAlias(alias, loc)
  optInFutureKeyword(path, alias, loc)

  return addStatement(new Ast.Import(path, alias, loc))
}

ImportOnly = _ 'import' ws path:(Reference / Variable) alias:Alias? _ {
  const loc = location()

  assertValidImportPath(path, loc)
  assertValidImportAlias(alias, loc)
  optInFutureKeyword(path, alias, loc)

  return new Ast.Import(path, alias, loc)
}

Alias = ws 'as' ws variable:Variable {
  return variable
}

// -----------------------------------------------------------------------------

LineBody = head:Expression tail:LineBodyExpressions* {
  if (!head.location) {
    throw new Error('Expected `head.location`:', text())
  }

  return [head].concat(tail)
}

LineBodyExpressions = _ ';' _ expression:Expression {
  return expression
}

// -----------------------------------------------------------------------------

Null = 'null' !IdentifierTail {
  return new Ast.Null(location())
}

// -----------------------------------------------------------------------------

Number = '-'? (Float / Integer) {
  return new Ast.Number(parseFloat(text()), location())
}

Exponent = 'e'i [+-]? DIGIT+

ExponentFloat = (PointFloat / Integer) Exponent

Float = ExponentFloat / PointFloat

Fraction = '.' DIGIT+

Integer = '0' / ([1-9] DIGIT*)

PointFloat = Integer? Fraction

// -----------------------------------------------------------------------------

Object = '{' _ head:Property? tail:Properties* _ ','? _ '}' {
  const loc = location()

  if (!head) {
    return new Ast.Object(undefined, loc)
  }

  return new Ast.Object([head].concat(tail), loc)
}

ObjectComprehension = '{' _ name:Term _ ':' _ value:Term _ '|' _ body:BlockBody _ '}' {
  return new Ast.ObjectComprehension(name, value, body, location())
}

Property = name:RecursiveExpression _ ':' _ value:RecursiveExpression {
  return [name, value]
}

Properties = _ ',' _ property:Property {
  return property
}

// -----------------------------------------------------------------------------
// Output

LhsOutput = term:Term _ operator:(':=' / '=') _ {
  const loc = location()
  const name = new Ast.Variable(Ast.Builtin.get(operator).name, loc)
  const reference = new Ast.Reference(name, loc)
  return {operator: reference, term}
}

RhsOutput = _ operator:(':=' / '=') _ term:Term {
  const loc = location()
  const name = new Ast.Variable(Ast.Builtin.get(operator).name, loc)
  const reference = new Ast.Reference(name, loc)
  return {operator: reference, term}
}

// -----------------------------------------------------------------------------

Package = 'package' ws path:(Reference / Variable) {
  return addStatement(new Ast.Package(path, location()))
}

PackageOnly = _ Package . * {
  return astModule
}

// -----------------------------------------------------------------------------

ParseError = . {
  if (parseErrors.length > 0) {
    throw parseErrors[0]
  }

  const message = `Unexpected token \`${text()}\``
  throw new Errors.ParserSyntaxError(message, location())
}

// -----------------------------------------------------------------------------

Reference = head:UnmangledVariable _ tail:(References)+ {
  return new Ast.Reference([head].concat(tail), location())
}

ReferenceOnly = _ head:UnmangledVariable _ tail:(References)* _ {
  return new Ast.Reference([head].concat(tail), location())
}

References = DotReference / BracketReference

BracketReference = '[' name:(RecursiveExpression) ']' {
  return name
}

DotReference = '.' variable:UnmangledVariable {
  // Convert `variable` into a string because 'foo.bar.baz' is equivalent to
  // 'foo["bar"]["baz"]'.
  return new Ast.String(variable.value, variable.location)
}

// -----------------------------------------------------------------------------

Rule = DefaultRule / NormalRule / BodylessRule / BodyRule

KeywordsOptIn = &{
  Object.keys(KEYWORDS).forEach((key) => {
    KEYWORDS[key] = true
  })
}

RuleOnly = _ rule:(KeywordsOptIn / Rule) _ {
  return rule
}

DefaultRule = 'default' ws name:(Reference / UnmangledVariable) value:RuleHeadOutput {
  const loc = location()
  const head = new Ast.RuleHead(name, null, value, loc)
  const body = Ast.RuleBody.makeTrue(loc)

  return addStatement(new Ast.Rule(head, body, loc).setDefault(true))
}

NormalRule = head:RuleHead _ body:RuleIfBody extensions:RuleExtension* {
  if (!head) {
    return
  }

  let rule = new Ast.Rule(head, body, location())
  addStatement(rule)

  const hasExtensions = extensions?.[0] != null

  if (hasExtensions) {
    const isPartial = head.getKind() !== Ast.RuleHead.Kind.COMPLETE
    const isOrdered = !!extensions[0].term

    // Because all extensions are parsed in a single pass, the root rule’s `end`
    // location is parsed as the end of the entire rule chain. In order to use
    // the correct `end` location, replace the root rule’s `end` with the first
    // extension’s stashed `start` (which is different than the extension’s
    // rule-head `start` location).
    rule.location = Ast.makeLocation({end: extensions[0].start, start: rule.location.start})

    extensions.forEach((extension) => {
      const {location} = extension

      if (extension.term) {
        if (isPartial) {
          const message = 'Unexpected `else` after partially defined object or set'
          throw new Errors.ParserSyntaxError(message, location)
        }

        if (!isOrdered) {
          const message = 'Unexpected `else`'
          throw new Errors.ParserSyntaxError(message, location)
        }

        const elseRule = makeElseRule(head, extension)
        rule.setElse(elseRule)
        rule = elseRule
      } else {
        if (isOrdered) {
          const message = 'Expected `else`'
          throw new Errors.ParserSyntaxError(message, location)
        }

        const head = rule.head.clone()
        rule = new Ast.Rule(head, extension.body, location)
        addStatement(rule)
      }
    })
  }

  return rule
}

BodylessRule = name:(Reference / UnmangledVariable) _ maybeArguments:ArgumentList? _ keyword:(Contains) _ value:RecursiveExpression {
  const loc = location()

  const head = new Ast.RuleHead(name, maybeArguments, new Ast.Set(value, value.location), loc)
  const body = Ast.RuleBody.makeTrue(loc)

  return addStatement(new Ast.Rule(head, body, loc))
}

BodyRule = body:LineBody {
  if (body.length > 1) {
    const message = '`;` not allowed outside of rule or function body'
    throw new Errors.ParserSyntaxError(message, location())
  }

  return addStatement(makeRuleFromExpression(body[0]))
}

RuleHead = name:(Reference / UnmangledVariable) _ maybeArguments:ArgumentList? _ maybeKey:(ContainsKey / Key)? maybeValue:RuleHeadOutput? {
  const loc = location()

  if (name instanceof Ast.Reference) {
    const tail = name.value.at(-1) 

    if (tail instanceof Ast.Variable) {
      maybeKey = tail
      name = name.slice(0, -1)

      // name with a single value is not a Reference anymore, extract name out of the array
      if (name.value.length === 1) {
        [name] = name.value
      }
    }
  }

  if (maybeArguments && maybeKey) {
    const type = `${Ast.Object.TYPE}/${Ast.Set.TYPE} ${Ast.Rule.TYPE}`
    const message = `Partial ${type} definitions do not accept arguments`
    throw new Errors.ParserSyntaxError(message, loc)
  }

  let value
  if (maybeValue) {
    if (maybeKey) {
      const type = maybeKey.constructor

      if (type !== Ast.Reference && type !== Ast.String && type !== Ast.Variable) {
        const types = `${Ast.Reference.TYPE}, ${Ast.String.TYPE}, or ${Ast.Variable.TYPE}`
        const message = `Key must be a ${types} type, not ${type}`
        throw new Errors.ParserTypeError(message, loc)
      }
    }

    value = maybeValue
  } else if (!maybeKey) {
    value = new Ast.Boolean(true, loc)
  }

  return new Ast.RuleHead(name, maybeArguments ?? maybeKey, value, loc)
}

RuleIfBody = terms:((RuleIf? _ RuleBody) / (RuleIf _ RuleLineBody)) {
  const [, , body] = terms
  return body
}

RuleIf = "if" {
  if (!KEYWORDS['if']) {
    const importKeyword = 'future.keywords.if'
    const message = `\`import ${importKeyword}\` opt-in to use \`if\` keyword`
    throw new Errors.ParserKeywordError('if', importKeyword, message, location())
  }
}

RuleHeadOutput = _ operator:AssignOperator _ expression:Expression {
  const term = Array.isArray(expression.terms) ? expression : expression.terms
  return {operator, term, location: location()}
}

RuleBody = '{' _ body:BlockBody? _ '}' {
  if (!body) {
    const message = 'Rule body must not be empty'
    throw new Errors.ParserSyntaxError(message, location())
  }

  return body
}

RuleLineBody = lineBody:LineBody {
  return new Ast.RuleBody(lineBody, location())
}

RuleExtension = _ extension:(Else / EmptyElse / Disjunction) {
  // The root rule’s parsed `end` location corresponds to the end of the entire
  // rule chain because all extensions are parsed in a single pass. Stash the
  // current `start` location so that we can use it to fix up the root rule’s
  // location later.
  return {...extension, start: location().start}
}

Disjunction = body:RuleBody {
  return {body, location: location()}
}

Else = 'else' _ maybeTerm:RuleHeadOutput? _ body:RuleBody {
  const loc = location()
  return {term: maybeTerm || new Ast.Boolean(true, loc), body, location: loc}
}

EmptyElse = 'else' _ term:RuleHeadOutput {
  return {term, body: Ast.RuleBody.makeTrue(location()), location: location()}
}

Key = _ '[' _ key:RecursiveExpression _ ']' _ {
  return key
}

ContainsKey = _ Contains _ key:RecursiveExpression _ {
  return key
}

Contains = "contains" {
  if (!KEYWORDS['contains']) {
    const importKeyword = 'future.keywords.contains'
    const message = `\`import ${importKeyword}\` opt-in to use \`contains\` keyword`
    throw new Errors.ParserKeywordError('contains', importKeyword, message, location())
  }

  return 'contains'
}

ArgumentList = '(' _ head:RecursiveExpression? tail:Arguments* _ ','? _ ')' {
  return head ? [head].concat(tail) : null
}

Arguments = _ ',' _ argument:RecursiveExpression {
  return argument
}

// -----------------------------------------------------------------------------

Set = EmptySet / SetLiteral

EmptySet = 'set(' _ ')' {
  return new Ast.Set(undefined, location())
}

SetLiteral = '{' _ head:RecursiveExpression tail:RecursiveExpressions* _ ','? _ '}' {
  return new Ast.Set([head].concat(tail), location())
}

SetComprehension = '{' _ term:Term _ '|' _ body:BlockBody _ '}' {
  return new Ast.SetComprehension(term, body, location())
}

// -----------------------------------------------------------------------------

String = QuotedString / RawString

QuotedString = '"' characters:Character* '"' {
  return new Ast.String(characters.join(''), location())
}

RawString = '`' characters:([^`])* '`' {
  return new Ast.String(characters.join(''), location())
}

Character = UnescapedCharacter / EscapeSequence

EscapeSequence = '\\' sequence:EscapedCharacter {
  return sequence
}

EscapedCharacter = '"' / '\\' / '/' / Backspace / CarriageReturn / FormFeed
  / NewLine / Tab / UnicodeSequence

Backspace = 'b' {
  return '\b'
}

CarriageReturn = 'r' {
  return '\r'
}

FormFeed = 'f' {
  return '\f'
}

HexDigit = [0-9A-Fa-f]

NewLine = 'n' {
  return '\n'
}

Tab = 't' {
  return '\t'
}

UnescapedCharacter = [^\x00-\x1F"\\]

UnicodeSequence = 'u' hexDigits:$(HexDigit HexDigit HexDigit HexDigit) {
  return String.fromCharCode(parseInt(hexDigits, 16))
}

// -----------------------------------------------------------------------------

Term = head:(Call / Comprehension / Reference / Value / Variable) tail:(References)* {
  if (tail.length > 0) {
    return new Ast.Reference([head].concat(tail), location())
  }

  return head
}

// -----------------------------------------------------------------------------

True = 'true' !IdentifierTail {
  return new Ast.Boolean(true, location())
}

// -----------------------------------------------------------------------------

Collection = Array / Object / Set

Comprehension = ArrayComprehension / ObjectComprehension / SetComprehension

Value = Collection / False / Null / Number / String / True

ValueOnly = _ value:Value _ {
  return value
}

// -----------------------------------------------------------------------------

Variable = matches:CheckedVariable {
  return mangleVariable(matches[0], location())
}

Variables = _ ',' _ variable:Variable {
  return variable
}

UnmangledVariable = matches:CheckedVariable {
  return matches[0]
}

// Prevent keywords from being used as variable names.
CheckedVariable = variable:UncheckedVariable &{
  const keyword = variable.value
  const isFutureKeyword = Ast.FUTURE_KEYWORDS.includes(keyword)

  if (isFutureKeyword && !KEYWORDS[CHECK_FUTURE_KEYWORDS]) {
    return true
  }
  
  const isKeyword = !!KEYWORDS[keyword]

  if (isKeyword) {
    const message = `Unexpected \`${keyword}\` keyword`
    const error = new Errors.ParserSyntaxError(message, location())
    parseErrors.push(error)
  }

  return !isKeyword
}

UncheckedVariable = IdentifierHead IdentifierTail* {
  return new Ast.Variable(text(), location())
}

IdentifierHead = ALPHA / [_]

IdentifierTail = IdentifierHead / DIGIT

// -----------------------------------------------------------------------------

ALPHA = [A-Za-z]

DIGIT = [0-9]

EOF = !.

ws 'whitespace' = [ \n\r\t]+

_ 'whitespace' = ([ \n\r\t] / Comment)*

// -----------------------------------------------------------------------------
