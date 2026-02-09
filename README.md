# wmao

Work my ass off. Manage multiple coding agents.

- Backend is in Go, frontend in SolidJS.
- Expects [md](https://github.com/maruel/md) to be in `$PATH`.
- Requires docker to be installed.

## Installation

```bash
go install github.com/maruel/wmao/backend/cmd/wmao@latest
```

### systemd user service

Install the unit file and enable it:

```bash
mkdir -p ~/.config/systemd/user
cp contrib/wmao.service ~/.config/systemd/user/
systemctl --user daemon-reload
systemctl --user enable --now wmao
```

Edit `~/.config/systemd/user/wmao.service` to adjust `-root`, `-http`, and
`-logs` flags to match your setup.

View logs:

```bash
journalctl --user -u wmao -f
```
