# aws-ec2logshipper

`aws-ec2logshipper` is a small daemon that tails a local log file on an
EC2 instance and forwards each line to AWS CloudWatch Logs. It parses
timestamps from log lines, batches events, and sends them through the AWS SDK
for Go v2.

## Features

- Tails a local log file and follows log rotation.
- Extracts timestamps using a configurable Go time layout and regex.
- Batches and flushes log events to CloudWatch Logs.
- Resolves the EC2 instance ID via IMDSv2 when no stream name is provided.
- Ships as an RPM with a systemd unit.

## Prerequisites

- Go 1.26 or later (declared in `go.mod` and the RPM `BuildRequires`).
- `rpmbuild` and related RPM build tools for packaging.
- A systemd-based distribution when installing the RPM.
- AWS credentials on the host, such as an IAM instance profile or
  `~/.aws/credentials`.

## Build

Compile the local binary:

```bash
make build
```

Or directly with `go build`:

```bash
go build -o ec2logshipper .
```

## Run

Copy the example configuration and edit it for your environment:

```bash
sudo mkdir -p /etc/aws-ec2logshipper
sudo cp config.json.example /etc/aws-ec2logshipper/config.json
# edit /etc/aws-ec2logshipper/config.json
./ec2logshipper -config /etc/aws-ec2logshipper/config.json
```

The binary defaults to `-config /etc/aws-ec2logshipper/config.json`.

## Test log generator

The `loggen` utility produces configurable volumes of log output:

```bash
go run ./loggen
go run ./loggen -rate 100 -burst 10 -count 10000 -output /tmp/test.log
```

It emits lines in the `Jan 02 15:04:05` format used by the example config.

## Build the RPM

The package version is read from the `VERSION` file. To build the RPM:

```bash
make rpm
```

Artifacts are written to:

- `build/rpmbuild/RPMS/x86_64/aws-ec2logshipper-<version>-1.x86_64.rpm`
- `build/rpmbuild/SRPMS/aws-ec2logshipper-<version>-1.src.rpm`

To bump the version, edit `VERSION` and run `make rpm` again.

## Install the RPM

```bash
sudo rpm -ivh build/rpmbuild/RPMS/x86_64/aws-ec2logshipper-0.1.0-1.x86_64.rpm
sudo systemctl enable aws-ec2logshipper
sudo systemctl start aws-ec2logshipper
sudo systemctl status aws-ec2logshipper
```

The RPM installs:

- `/usr/bin/ec2logshipper`
- `/usr/lib/systemd/system/aws-ec2logshipper.service`
- `/etc/aws-ec2logshipper/config.json`

## Configuration

The `config.json` fields are:

| Field | Default | Description |
|---|---|---|
| `log_file` | `/var/log/messages` | Path to the local log file to tail. |
| `log_group` | `/ec2/application` | CloudWatch Logs group name. |
| `log_stream` | `""` | Stream name; empty means use the EC2 instance ID. |
| `timestamp_layout` | `"Jan 02 15:04:05"` | Go time layout for parsing the timestamp. |
| `timestamp_regex` | `"^(\\w{3} \\d{1,2} \\d{2}:\\d{2}:\\d{2})"` | Regex whose first capture group contains the timestamp. |
| `region` | `"us-east-1"` | AWS region for CloudWatch Logs. |
| `imdsv2` | `true` | Use IMDSv2 when resolving the instance ID. |
| `batch_max_size` | `100` | Maximum events per `PutLogEvents` call. |
| `flush_interval_ms` | `500` | Flush interval for the pending batch. |
| `create_log_group` | `true` | Create the log group if it does not exist. |
| `create_log_stream` | `true` | Create the log stream if it does not exist. |

## Continuous Integration

A GitHub Actions workflow in `.github/workflows/rpm.yml` builds the RPM
automatically whenever a `v*` Git tag is pushed. The tag without its leading
`v` must match `VERSION`. Binary and source RPMs are uploaded as workflow
artifacts and attached to the corresponding GitHub Release for durable
downloads by EC2 instances. The workflow can also be run manually to produce
workflow artifacts without creating a release.

## Development

Common make targets:

- `make build` – compile the binary.
- `make test` – run `go test ./...`.
- `make clean` – remove the binary and `build` directory.
- `make rpm` – build the source and binary RPMs.

Before committing, run `go vet ./...` and `gofmt -w .`.
