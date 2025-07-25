function AstSet(value, location) {
  if (value != null) {
    this.value = new Set(Array.isArray(value) ? value : [value])
  } else {
    this.value = new Set()
  }

  this.location = location
}

AstSet.TYPE = 'set'

AstSet.prototype.toJSON = function () {
  return {
    type: AstSet.TYPE,
    value: [...this.value.values()],
    location: this.location
  }
}

AstSet.prototype.valueOf = function () {
  const set = new Set()
  this.value.forEach((x) => set.add(x.valueOf()))
  return set
}

export default AstSet
