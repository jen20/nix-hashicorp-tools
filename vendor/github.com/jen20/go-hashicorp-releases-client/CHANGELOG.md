# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [v1.0.0] - 2025-02-25

### Features

- A `Client` may be constructed with a custom URL, or a custom `http.Client`.
- A `Client` may be constructed with a custom User-Agent string.
- The `Products` function may be used to obtain a list of all products tracked by the Releases API.
- The `Releases` function may be used to obtain an iterator over releases of a given product. 
- The `ReleasesPage` function may be used to obtain an iterator over pages of release information of a given product.
- The `Release` function may be used to obtain metadata about a single release of a given product.
- The `LatestRelease` function may be used to obtain metadata about the latest release of a given product.