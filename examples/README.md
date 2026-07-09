# Examples

Runnable pure-Ruby usage of the `mustache` library, verified under the [rbgo](https://github.com/go-embedded-ruby/ruby) interpreter.

```sh
rbgo examples/mustache_usage.rb
```

| File | Shows |
| ---- | ----- |
| `mustache_usage.rb` | `Mustache.render` one-shot rendering, String/Symbol keys, HTML-escaped `{{var}}` vs raw `{{{var}}}`, hash/array/inverted sections, lambda sections, the `Mustache.new` view API, and `Mustache::ParseError`. |
