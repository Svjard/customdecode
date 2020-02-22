# customdecode [![Travis-CI](https://travis-ci.org/Svjard/customdecode.svg)](https://travis-ci.org/Svjard/customdecode) [![GoDoc](https://godoc.org/github.com/Svjard/customdecode?status.svg)](https://godoc.org/github.com/Svjard/customdecode)

`customdecode` is a Go package for populating structs from any custom decoder function, e.g. AWS Secrets, Files, Environment Variables, etc...

`customdecode` uses struct tags to map variables derived from any custom function to fields, allowing you you use any names you want for custom variables.
`customdecode` will recurse into nested structs, including pointers to nested structs, but it will not allocate new pointers to structs.

## API

Full API docs are available on
[godoc.org](https://godoc.org/github.com/Svjard/customdecode).

Define a struct with `custom` struct tags:

```go
type Config struct {
    Hostname  string `custom:"SERVER_HOSTNAME,default=localhost"`
    Port      uint16 `custom:"SERVER_PORT,default=8080"`

    AWS struct {
        ID        string   `custom:"AWS_ACCESS_KEY_ID"`
        Secret    string   `custom:"AWS_SECRET_ACCESS_KEY,required"`
        SnsTopics []string `custom:"AWS_SNS_TOPICS"`
    }

    Timeout time.Duration `custom:"TIMEOUT,default=1m,strict"`
}
```

Fields *must be exported* (i.e. begin with a capital letter) in order for `customdecode` to work with them.  An error will be returned if a struct with no exported fields is decoded (including one that contains no `custom` tags at all).
Default values may be provided by appending ",default=value" to the struct tag. Required values may be marked by appending ",required" to the struct tag. Strict values may be marked by appending ",strict" which will return an error on Decode if there is an error while parsing.

Then call `customdecode.Decode`:

```go
func EnvDecode(s string) string {
  return os.Getenv(s)
}

var cfg Config
err := customdecode.Decode(&cfg, EnvDecode)
```

If you want all fields to act `strict`, you may use `customdecode.StrictDecode`:

```go
func EnvDecode(s string) string {
  return os.Getenv(s)
}

var cfg Config
err := customdecode.StrictDecode(&cfg, EnvDecode)
```

All parse errors will fail fast and return an error in this mode.

## Supported types

* Structs (and pointer to structs)
* Slices of below defined types, separated by semicolon
* `bool`
* `float32`, `float64`
* `int`, `int8`, `int16`, `int32`, `int64`
* `uint`, `uint8`, `uint16`, `uint32`, `uint64`
* `string`
* `time.Duration`, using the [`time.ParseDuration()` format](http://golang.org/pkg/time/#ParseDuration)
* `*url.URL`, using [`url.Parse()`](https://godoc.org/net/url#Parse)
* Types those implement a `Decoder` interface

## Custom `Decoder`

If you want a field to be decoded with custom behavior, you may implement the interface `Decoder` for the filed type.

```go
type Config struct {
  IPAddr IP `env:"IP_ADDR"`
}

type IP net.IP

// Decode implements the interface `envdecode.Decoder`
func (i *IP) Decode(repl string) error {
  *i = net.ParseIP(repl)
  return nil
}
```

`Decoder` is the interface implemented by an object that can decode a custom variable string representation of itself.
