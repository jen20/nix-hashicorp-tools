# HashiCorp Releases API Client for Go

This repository contains a client library for V1 of the [HashiCorp Releases API][hashicorp-api].
The library is dependency-free, but requires Go 1.23, since it returns [iterators][go-iterators] either for records or pages of records.

## Documentation

Complete documentation for this library is available via [godoc][godoc].

Note that [functional options][functional-options] may be supplied when creating a client for the following purposes:
- Using a custom `*http.Client` for making requests,
- Overriding the URL of the service (as is used with `httptest` in integration tests, for example),
- Changing the value of the `User-Agent` header sent with each request, or omitting the header.

## Development & Contributions

This repository contains a [Nix][nix] flake which will install the various tools such as the Go compiler, formatter and linter.
When used with [direnv][direnv], this will be done automatically upon entering the repository directory.

Pull requests which do not add external dependencies are accepted. 

## License

This library is licensed under the Mozilla Public License V2.

[hashicorp-api]: https://releases.hashicorp.com/docs/api/v1/
[go-iterators]: https://go.dev/blog/range-functions
[godoc]: https://pkg.go.dev/github.com/jen20/go-hashicorp-releases-client
[functional-options]: https://dave.cheney.net/2014/10/17/functional-options-for-friendly-apis
[nix]: https://nixos.org
[direnv]: https://direnv.net