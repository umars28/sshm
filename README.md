# sshm

TUI ringan (Go) untuk mengelola koneksi SSH bernama. Daftar koneksi, connect
dengan satu enter, custom key per koneksi, lock ssh-agent saat sleep/restart,
dan viewer `authorized_keys` (lokal & remote).

## Kenapa

`ssh` polos cuma hostname. `sshm` kasih daftar bernama ("ec2-project-a"),
masing-masing dengan key sendiri, dan tetap kompatibel dengan `ssh <nama>` di
shell mana pun — karena profil di-render ke blok terkelola di `~/.ssh/config`.

## Bangun

```bash
go build -o sshm .      # mengunduh charmbracelet/bubbletea, bubbles, lipgloss
```

Butuh Go 1.22+. (Dependency TUI di-pin ke versi yang kompatibel: bubbletea
v0.27, bubbles v0.20, lipgloss v0.13.)

## Pakai

```bash
sshm                 # buka TUI
sshm sync            # tulis ulang blok ~/.ssh/config dari store
sshm lock            # buang semua key dari agent (passphrase diminta lagi)
sshm guard           # penjaga: lock agent otomatis saat sleep
sshm guard install   # pasang service (systemd-user / launchd)
```

Tombol di TUI: `enter` connect · `a` add · `e` edit · `d` delete ·
`l` load key ke agent · `x` lock agent · `r` baca authorized_keys remote ·
`R` baca lokal · `q` quit.

## Model keamanan ("masukin password lagi habis sleep/restart")

Ini persis perilaku **ssh-agent + key ber-passphrase**:

- **Restart** — agent mati saat shutdown, semua key hilang. Pemakaian key
  ber-passphrase berikutnya otomatis minta passphrase lagi. Gratis.
- **Sleep** — tidak otomatis. `sshm guard` dengarkan event suspend OS lalu
  jalankan `ssh-add -D`, jadi setelah bangun, koneksi pertama minta passphrase
  lagi.
  - Linux: subscribe sinyal `PrepareForSleep` dari logind via `dbus-monitor`.
  - macOS: pasang launchd plist + tool `sleepwatcher` yang memanggil `sshm lock`.

Alur: tekan `l` di sebuah profil → `ssh-add` minta passphrase sekali → koneksi
berikutnya dalam sesi itu tanpa passphrase, sampai sleep/restart me-lock lagi.

## Penyimpanan & keamanan file

- Profil: `~/.config/sshm/profiles.json` (0600).
- `~/.ssh/config`: hanya blok di antara penanda `# >>> sshm managed >>>` yang
  disentuh; isi tulisan tanganmu (sebelum & sesudah blok) tidak diubah. Dua
  backup dibuat: `.sshm.orig` (config asli sebelum sshm, ditulis sekali & tidak
  pernah ditimpa) dan `.sshm.bak` (state sebelum perubahan terakhir). Sync yang
  tidak mengubah apa pun tidak menyentuh file sama sekali.
- `sshm` tidak pernah menyimpan passphrase; semua passphrase ditangani
  langsung oleh `ssh`/`ssh-add`.

## authorized_keys

- `R` (lokal): baca `~/.ssh/authorized_keys`.
- `r` (remote): `ssh <nama> cat ~/.ssh/authorized_keys` (BatchMode, gagal cepat
  bila perlu password). Parser menampilkan tipe key, ringkasan, komentar, dan
  options (mis. `command="..."`).

## Status implementasi

Logika inti (store profil, sync `~/.ssh/config`, parser authorized_keys,
kontrol agent) sudah dites unit. Lapisan TUI memakai Bubble Tea. Penjaga sleep
bersifat OS-spesifik dan perlu diuji di perangkat nyata (Linux dbus / macOS
launchd).

## Roadmap

- Import otomatis Host yang sudah ada di `~/.ssh/config` jadi profil.
- Edit `authorized_keys` remote (tambah/cabut key) dari TUI.
- Grouping berdasarkan tag; pencarian/filter di daftar.
- ProxyJump / bastion per profil.
