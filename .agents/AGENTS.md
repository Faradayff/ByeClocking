# ByeClocking Project Rules

These rules apply to all tasks in this workspace.

## Project Information

**ByeClocking** is a Go application designed to automatically clock in and out at work while simulating natural,
human-like patterns (unpunctuality, random lunch durations).

- **Language**: Go
- **Logging**: Uses standard library `log/slog` (outputs to stdout and `ByeClocking.log`).
- **Configuration**: Uses `config.json` for all settings (account, platform, schedule, unpunctuality parameters).
- **Core Logic**: Runs an infinite loop calculating the next action time with randomized delays and uses timers to
  execute actions.
- **Extensibility**: Prepared to support multiple clocking platforms (in the `clients` directory).

## Style Guidelines

- Follow standard Go formatting (`gofmt`).
- Use structured logging with `slog` for all events instead of standard `fmt.Print` or `log.Print`.
- Configuration settings should map exactly to JSON in `config.go` and have strict validation logic.
- Make use of standard Go concurrency patterns (`context.Context`, `time.Timer`) for scheduling tasks.

## Behavioral Constraints

- When adding new features, respect the core philosophy of simulating human unpunctuality via random delays.
- Any new configuration parameters must be added to `config.json` and validated in `config.go`.
- Avoid hardcoding times or secrets; everything must go through `config.json`.
