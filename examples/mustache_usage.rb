# frozen_string_literal: true

require "mustache"

# One-shot rendering: interpolate a context into a template. String and Symbol
# keys both resolve, so {{name}} matches "name" => ... or name: ...
puts Mustache.render("Hello {{name}}!", "name" => "Ada")   # => Hello Ada!
puts Mustache.render("Hello {{name}}!", name: "Ada")       # => Hello Ada!

# {{var}} HTML-escapes; {{{var}}} (or {{&var}}) emits the value untouched.
puts Mustache.render("{{v}}", v: "<b>&</b>")    # => &lt;b&gt;&amp;&lt;/b&gt;
puts Mustache.render("{{{v}}}", v: "<b>&</b>")  # => <b>&</b>

# Sections: a truthy value renders the block once against that value...
puts Mustache.render("{{#user}}{{name}} ({{age}}){{/user}}",
                     user: { name: "Bo", age: 30 })       # => Bo (30)

# ...an Array iterates the block, with {{.}} for the current element...
puts Mustache.render("{{#items}}[{{.}}]{{/items}}", items: %w[a b c]) # => [a][b][c]

# ...and an inverted section renders only when the value is empty/false.
puts Mustache.render("{{^items}}none{{/items}}", items: []) # => none

# A lambda section receives the raw block text and returns the replacement.
puts Mustache.render("{{#loud}}hi{{/loud}}", loud: ->(t) { t.upcase }) # => HI

# Class-based view API: build a reusable view, then render with a context.
view = Mustache.new("{{greeting}}, {{name}}!")
puts view.render(nil, greeting: "Hi", name: "Zoe")         # => Hi, Zoe!

# A malformed template raises Mustache::ParseError.
begin
  Mustache.render("{{#x}}unclosed", {})
rescue Mustache::ParseError => e
  puts "ParseError: #{e.message}"
end
