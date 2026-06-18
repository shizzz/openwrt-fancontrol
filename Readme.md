# openwr-fancontrol

A production-ready PWM fan control daemon for OpenWrt Linux, written in Go.

## Features

- **PID control** with anti-windup, derivative filter, and output clamping
- **Fixed** and **table** mode stubs ready for extension
- **sysfs-based** hardware access (`/sys/class/thermal`, `/sys/class/hwmon`)
- **Dry-run mode** — logs intended PWM without writing to hardware
- **procd**-compatible init script (no systemd)
- **Zero external dependencies** — pure Go stdlib
- Graceful SIGTERM shutdown
- Structured stdout logging (readable via `logread` on OpenWrt)

## Directory layout

```
openwr-fancontrol/
├── cmd/fancontrol/main.go          # entrypoint
├── internal/
│   ├── config/config.go            # env-based config loader
│   ├── control/loop.go             # main control loop + mode dispatch
│   ├── pid/pid.go                  # PID controller
│   ├── pid/pid_test.go
│   ├── sysfs/sysfs.go              # hardware access layer
│   └── sysfs/sysfs_test.go
├── etc/init.d/fancontrol           # procd init script
├── openwrt/Makefile                # OpenWrt SDK package Makefile
├── Makefile                        # native + cross-compile targets
└── go.mod
```

## Build

### Native (for testing)
```sh
make build
```

### Cross-compile for OpenWrt arm64
```sh
make ARCH=arm64 build
```

### Cross-compile for OpenWrt MIPS (little-endian, soft-float)
```sh
make ARCH=mipsle build
```

### Run tests
```sh
make test
```

## Configuration

All configuration is via environment variables (set in the procd init script):

| Variable | Default | Description |
|---|---|---|
| `FANCTL_THERMAL_PATH` | `/sys/class/thermal/thermal_zone0/temp` | Thermal sensor path |
| `FANCTL_PWM_PATH` | `/sys/class/hwmon/hwmon0/pwm1` | PWM output path |
| `FANCTL_PWM_ENABLE_PATH` | `/sys/class/hwmon/hwmon0/pwm1_enable` | PWM enable path |
| `FANCTL_MODE` | `pid` | Control mode: `pid`, `fixed`, `table` |
| `FANCTL_SETPOINT` | `55.0` | Target temperature in °C |
| `FANCTL_KP` | `2.0` | PID proportional gain |
| `FANCTL_KI` | `0.5` | PID integral gain |
| `FANCTL_KD` | `1.0` | PID derivative gain |
| `FANCTL_MIN_PWM` | `0` | Minimum PWM output (0–255) |
| `FANCTL_MAX_PWM` | `255` | Maximum PWM output (0–255) |
| `FANCTL_INTERVAL_SEC` | `1.0` | Control loop interval in seconds |
| `FANCTL_FIXED_PWM` | `128` | PWM value for fixed mode |
| `FANCTL_DRY_RUN` | `false` | Log without writing to sysfs |
| `FANCTL_DEBUG` | `false` | Enable PID term debug logging |

## OpenWrt installation

1. Copy the binary to the router:
   ```sh
   scp fancontrol root@192.168.1.1:/usr/sbin/fancontrol
   ```

2. Install the init script:
   ```sh
   scp etc/init.d/fancontrol root@192.168.1.1:/etc/init.d/fancontrol
   ssh root@192.168.1.1 chmod +x /etc/init.d/fancontrol
   ```

3. Edit `/etc/init.d/fancontrol` on the router to match your board's sysfs paths.

4. Enable and start:
   ```sh
   /etc/init.d/fancontrol enable
   /etc/init.d/fancontrol start
   ```

5. View logs:
   ```sh
   logread | grep fancontrol
   ```

## Extending

| Feature | Where to add |
|---|---|
| UCI config support | `internal/config/config.go` — add a `LoadUCI()` function |
| ubus API | new `internal/ubus/` package exposing status/control methods |
| Table mode | `internal/control/loop.go` `tableController.Compute()` |
| LuCI page | separate `luci-app-fancontrol` package |
| Multiple zones | extend `Config` with `[]ZoneConfig`; loop over them in `control` |