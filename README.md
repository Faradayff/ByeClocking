# ByeClocking

ByeClocking is a Go application designed to automate clock-ins and clock-outs at work by naturally simulating human
patterns. By introducing random delays (unpunctuality) and variable durations for lunch breaks, it avoids looking like
an automated script.

## How does it work?

The application works via an infinite loop that constantly calculates the exact time for the next action (clocking in,
lunch break, returning from lunch, or clocking out) and uses timers (`time.Timer`) to execute the actions.

* **Human simulation:** Uses configuration parameters to add random delays (`unpunctuality`, `leave_unpunctuality`,
  `lunch_unpunctuality`) to the base times.
* **Flexible configuration:** Allows configuring different schedules for Fridays (`friday_times`) and for summer (
  `summer_times` and `summer_period`).
* **Lunch break:** If configured, it randomly calculates the duration of the break between a minimum (
  `min_time_to_lunch`) and a maximum (`max_time_to_lunch`) of minutes.
* **Multiple platforms:** Its design allows integration with multiple clocking platforms. Currently, the only supported
  platform is `myteam2go`.
* **Security and Configuration:** Secure handling of credentials via environment variables (with the `BYECLOCKING_`
  prefix) separated from the general configuration.
* **Geolocation:** Optionally, latitude and longitude can be configured. If not provided, it will simulate that the
  browser denied geolocation.
* **Continuous Integration and Deployment (CI/CD):** Automated pipelines with GitHub Actions for tests, Go compilation,
  and multi-platform Docker image builds with image signing.

## Deployment

Since ByeClocking is designed to run continuously on a server, the recommended way to deploy it is using its pre-built *
*Docker** image with **Docker Compose**. There is no need to clone this repository.

### Option 1: Using the pre-built Docker image (Recommended)

1. Have Docker and Docker Compose installed.
2. Create a directory for ByeClocking and create a `docker-compose.yml` file inside it with the following content:
   ```yaml
   services:
     byeclocking:
       image: ghcr.io/faradayff/byeclocking:latest
       container_name: byeclocking
       restart: always
       volumes:
         - ./configs:/app/configs
         - ./logs:/app/logs
       env_file:
         - .env
   ```
3. Create a `configs` folder and add your `config.json` inside it (you can base it on the project's example
   configuration).
4. Start the container in the background:
   ```bash
   docker-compose up -d
   ```

### Option 2: Building locally from source

If you prefer to build the image yourself:

1. Have Docker and Docker Compose installed.
2. Clone this repository on your machine.
3. Copy the example configuration file and adjust it to your needs:
   ```bash
   cp configs/config.example.json configs/config.json
   ```
4. Run the following command in the root of the project to build the image and start the container:
   ```bash
   docker-compose up -d --build
   ```

### Configuration (`configs/config.json`)

Since you are not cloning the repository, you can create the `configs/config.json` file by copying the following example
and adjusting it to your preferences:

```json
{
  "clock_in": "08:00",
  "clock_out": "17:00",
  "unpunctuality": 10,
  "leave_unpunctuality": 20,
  "lunchtime": "14:00",
  "min_time_to_lunch": 15,
  "max_time_to_lunch": 40,
  "lunch_unpunctuality": 60,
  "friday_times": [
    "8:00",
    "15:00"
  ],
  "summer_times": [
    "8:00",
    "15:00"
  ],
  "summer_period": [
    "01/07",
    "31/08"
  ],
  "latitude": 0.0,
  "longitude": 0.0,
  "clocking_platform": ""
}
```

**Parameters explained:**

- `clock_in`: Theoretical clock-in time (e.g. "08:00").
- `clock_out`: Theoretical clock-out time (e.g. "17:00").
- `unpunctuality`: Maximum delay minutes when clocking in.
- `leave_unpunctuality`: Maximum delay minutes when clocking out.
- `lunchtime`: Theoretical start time for lunch.
- `min_time_to_lunch` / `max_time_to_lunch`: Minimum and maximum lunch duration in minutes.
- `lunch_unpunctuality`: Maximum delay minutes before lunch.
- `friday_times` / `summer_times`: Exceptional schedules.
- `clocking_platform`: Platform to use (e.g. "myteam2go").

Have in mind that if you don't leave margin to randomize the clocking times, you'll get a fixed behavior that may be
suspicious for your company.

### Environment Variables (Credentials)

**Never** put passwords in the JSON file. Credentials are injected via environment variables. You should use a `.env`
file in the same directory as your `docker-compose.yml`.

**Important:** The required environment variables depend entirely on the clocking platform (client) you are using.

#### Supported Platforms and their required variables:

**1. `myteam2go`** (Currently the only supported platform)

- `BYECLOCKING_ACCOUNT=your_username`
- `BYECLOCKING_PASSWORD=your_password`
- `BYECLOCKING_COMPANY_NAME=your_company`

*(Note: If new platforms are added in the future, they will specify their own required environment variables here).*

### Viewing the logs

The application logs will be saved locally in the `./logs/` directory (thanks to the configured volume) and can also be
viewed with:

```bash
docker-compose logs -f
```

## Contribution Guide

If you wish to contribute improvements to ByeClocking, please review and strictly follow these project rules:

1. **Code Style:** Follow the standard Go format (`gofmt`) and adhere strictly to all official Go styling and idiomatic
   guidelines (e.g., Effective Go).
2. **Logs:** Always use the structured `log/slog` library to log events instead of using `fmt.Print` or `log.Print`.
3. **Configuration:**
    - Any new parameter must be added to `configs/config.example.json`, and mapped exactly in the structure of
      `internal/config/config.go`.
    - Implement strict validation logic for any new field in the `validate()` method inside `config.go`.
    - **Zero hardcoded secrets**: All sensitive variables must go through the configuration or environment variables.
4. **Core Behavior:**
    - Maintain and respect the core philosophy of the project: simulating human behavior (unpunctuality).
    - Use standard Go concurrency patterns (`context.Context`, `time.Timer`) for task scheduling. Avoid active
      blocks/polling.
5. **Adding new Platforms:**
    - The application is designed to be extensible. If you implement a new clocking platform, add the corresponding
      client in the `internal/clients` directory.

---
