# Contributing to DevOps Terminal Dashboard

Thank you for your interest in contributing to the DevOps Terminal Dashboard! This document provides guidelines and instructions for contributing.

## Code of Conduct

By participating in this project, you agree to abide by the [Code of Conduct](CODE_OF_CONDUCT.md).

## How Can I Contribute?

### Reporting Bugs

Bug reports help us improve the dashboard. When reporting a bug, please include:

- A clear, descriptive title
- Steps to reproduce the issue
- Expected behavior
- Current behavior
- Screenshots if applicable
- Environment information (OS, terminal, version)

### Suggesting Features

We welcome feature suggestions! Please provide:

- A clear description of the feature
- The use case or problem it addresses
- Any implementation ideas you have

### Pull Requests

We actively welcome pull requests:

1. Fork the repository
2. Create a branch (`git checkout -b feature/amazing-feature`)
3. Make your changes
4. Run tests (`make test`)
5. Commit your changes (`git commit -m 'Add amazing feature'`)
6. Push to the branch (`git push origin feature/amazing-feature`)
7. Open a Pull Request

## Development Environment Setup

1. Install Go 1.21 or higher
2. Clone the repository
3. Install dependencies:

```bash
go mod download
```

## Project Structure

- `cmd/` - Application entry points
- `internal/` - Private application and library code
- `pkg/` - Public API code that can be imported by other applications
  - `pkg/collector/` - Data collectors
  - `pkg/config/` - Configuration system
  - `pkg/models/` - Data models
  - `pkg/ui/` - User interface code
- `tests/` - Integration and unit tests
- `docs/` - Documentation

## Coding Standards

- Follow standard Go code style and conventions
- Write clear, descriptive commit messages
- Include comments for non-obvious code
- Add tests for new functionality
- Ensure all tests pass before submitting a PR

## Testing

Run the test suite:

```bash
make test
```

Run specific tests:

```bash
go test ./path/to/package -run TestName
```

## Documentation

- Update documentation to reflect your changes
- Document new features or behavior changes
- Keep the API documentation up-to-date

## Review Process

- All submissions require review
- Changes may be requested before a PR is merged
- Be responsive to feedback and be prepared to make adjustments

## License

By contributing, you agree that your contributions will be licensed under the project's [MIT License](LICENSE). 