import Parser from '../parser'
import * as Ast from '../ast'

function AstReference(value, location) {
  if (value) {
    this.value = Array.isArray(value) ? value : [value]
  } else {
    this.value = []
  }

  this.location = location
}

AstReference.TYPE = 'reference'

AstReference.fromPathname = function (pathname, location = Ast.makeLocation()) {
  const segments = pathname.split('/')
  const value = segments.slice(1).reduce((a, x) => {
    a.push(new Ast.String(x))
    return a
  }, [new Ast.Variable(segments[0])])

  return new AstReference(value, location)
}

AstReference.isValid = function (segments) {
  return segments.length > 0 && AstReference.isValidIdentifier(segments[0])
}

AstReference.isValidIdentifier = function (identifier) {
  const s = `${identifier}`
  return !Ast.KEYWORDS.includes(s) && Ast.IDENTIFIER_REGEX.test(s)
}

AstReference.parse = function (id) {
  return Parser.parse(id, {startRule: 'ReferenceOnly'})
}

AstReference.makePlaceholder = function () {
  const placeholder = new AstReference([''], Ast.makeLocation())
  placeholder.isPlaceholder = true
  return placeholder
}

AstReference.stringify = function (segments) {
  let buffer = `${segments[0]}`

  for (let i = 1, ni = segments.length; i < ni; ++i) {
    const segment = segments[i]
    buffer += segment instanceof Ast.String
      ? AstReference.isValidIdentifier(segment)
        ? `.${segment}`
        : `["${`${segment}`.replace(/"/g, '\\"')}"]`
      : `[${segment}]`
  }

  return buffer
}

Object.defineProperty(AstReference.prototype, 'length', {
  get() {
    return this.value.length
  }
})

AstReference.prototype.hasRoot = function () {
  const [x] = this.value
  return Ast.isVariable(x) && x.isRoot()
}

AstReference.prototype.isRoot = function () {
  const [x] = this.value
  return this.length === 1 && Ast.isVariable(x) && x.isRoot()
}

AstReference.prototype.slice = function (from, to) {
  const location = Ast.makeLocation({
    start: this.value.at(from)?.location.start ?? this.location.start,
    end: this.value.at((to ?? 0) - 1)?.location.end ?? this.location.end
  })

  return new AstReference(this.value.slice(from, to), location)
}

AstReference.prototype.toPathname = function () {
  return this.value.join('/')
}

AstReference.prototype.toJSON = function () {
  return {
    type: AstReference.TYPE,
    value: this.value.slice(),
    location: this.location
  }
}

AstReference.prototype.toString = function () {
  return AstReference.stringify(this.value)
}

export default AstReference
