# openwrt-fancontrol

A production-ready PWM fan control daemon for OpenWrt Linux, written in Go.

## Features

- **PID control** with anti-windup, derivative filter, and output clamping
- **Multiple fans** — each with its own sensor, PWM output, control
  parameters, and enable flag, all driven concurrently from one process
- **Fixed** and **table** mode stubs ready for extension
- **UCI configuration** at `/etc/config/fancontrol`, with `FANCTL_*`
  env-var fallback when the UCI file is absent
- **sysfs-based** hardware access (`/sys/class/thermal`, `/sys/class/hwmon`)
- **Dry-run mode** — logs intended PWM without writing to hardware
- **procd**-compatible init script (no systemd)
- **Zero external dependencies** — pure Go stdlib
- Graceful SIGTERM shutdown
- Structured stdout logging (readable via `logread` on OpenWrt)

## Directory layout

```
openwrt-fancontrol/
├── cmd/fancontrol/main.go          # entrypoint; spawns one Loop per fan
├── internal/
│   ├── config/
│   │   ├── config.go               # multi-fan env + UCI loader
│   │   ├── config_test.go
│   │   └── uci.go                  # minimal UCI parser (multi-section)
│   ├── control/loop.go             # per-fan control loop + mode dispatch
│   ├── pid/pid.go                  # PID controller
│   ├── pid/pid_test.go
│   ├── sysfs/sysfs.go              # hardware access layer
│   └── sysfs/sysfs_test.go
├── etc/
│   ├── config/fancontrol           # sample UCI config (multi-fan)
│   └── init.d/fancontrol           # procd init script
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

The daemon supports multiple fans, each with its own sensor, PWM output,
and control parameters.  Sources are tried in this order:

1. **UCI** at `/etc/config/fancontrol` — one `config fancontrol '<name>'`
   section per fan.  When the UCI file exists, **env vars are ignored**.
2. **Environment variables** (`FANCTL_*`) — used only when no UCI file
   is present.  Creates a single fan named `default`.
3. Built-in defaults.

If no fans end up enabled, the daemon exits with an error.

### UCI (multi-fan)

Install the sample config and edit it for your board:

```sh
scp etc/config/fancontrol root@192.168.1.1:/etc/config/fancontrol
ssh root@192.168.1.1 vi /etc/config/fancontrol
```

Minimal two-fan example:

```
config fancontrol 'cpu'
	option enabled '1'
	option mode 'pid'
	option setpoint '60'
	option kp '2.0'
	option ki '0.5'
	option kd '1.0'
	option min_pwm '50'
	option max_pwm '255'
	option interval_sec '1.0'
	option thermal_path '/sys/class/thermal/thermal_zone0/temp'
	option pwm_path '/sys/class/hwmon/hwmon0/pwm1'
	option pwm_enable_path '/sys/class/hwmon/hwmon0/pwm1_enable'

config fancontrol 'case'
	option enabled '1'
	option setpoint '45'
	option thermal_path '/sys/class/thermal/thermal_zone1/temp'
	option pwm_path '/sys/class/hwmon/hwmon1/pwm2'
	option pwm_enable_path '/sys/class/hwmon/hwmon1/pwm2_enable'
```

Set `option enabled '0'` on any section to skip it without deleting the
config.  All options are documented in the sample file at
`etc/config/fancontrol`.

### Environment variables

Only consulted when the UCI file does not exist.  Each `FANCTL_*` maps
to the corresponding UCI option of the single `default` fan.

| Variable | Default | Description |
|---|---|---|
| `FANCTL_ENABLED` | `true` | Enable the env-driven fan (`false` exits the daemon) |
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

3. (Optional) install the sample UCI config and edit it for your board:
   ```sh
   scp etc/config/fancontrol root@192.168.1.1:/etc/config/fancontrol
   ssh root@192.168.1.1 vi /etc/config/fancontrol
   ```

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
| ubus API | new `internal/ubus/` package exposing status/control methods |
| Table mode | `internal/control/loop.go` `tableController.Compute()` |
| LuCI page | separate `luci-app-fancontrol` package |
| Multiple zones | extend `Config` with `[]ZoneConfig`; loop over them in `control` |