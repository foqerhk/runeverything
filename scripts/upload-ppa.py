#!/usr/bin/env python3
"""Build a signed Debian source package and upload to Launchpad PPA.

Usage:
  python3 scripts/upload-ppa.py                 # build + upload 0.1.0-1 to noble
  python3 scripts/upload-ppa.py --series jammy  # also build jammy variant
  python3 scripts/upload-ppa.py --dry-run       # build+sign only, no FTP

Requires: ~/.runeverything-gpg/{private.asc,passphrase.txt}
"""
from __future__ import annotations

import argparse
import hashlib
import io
import os
import re
import shutil
import subprocess
import sys
import tarfile
import tempfile
import time
from datetime import datetime, timezone
from ftplib import FTP
from pathlib import Path

try:
    from pgpy import PGPKey, PGPMessage
    from pgpy.constants import HashAlgorithm
except ImportError:
    print("error: pgpy required (pip install pgpy)", file=sys.stderr)
    raise SystemExit(2)

ROOT = Path(__file__).resolve().parents[1]
GPG_DIR = Path.home() / ".runeverything-gpg"
PPA_USER = "foqerhk"
PPA_NAME = "runeverything"
SOURCE = "runeverything"
MAINTAINER = "foqerhk <ruier09@qq.com>"


def sha1(p: Path) -> str:
    h = hashlib.sha1()
    h.update(p.read_bytes())
    return h.hexdigest()


def sha256(p: Path) -> str:
    h = hashlib.sha256()
    h.update(p.read_bytes())
    return h.hexdigest()


def md5(p: Path) -> str:
    h = hashlib.md5()
    h.update(p.read_bytes())
    return h.hexdigest()


def rfc2822_now() -> str:
    return datetime.now(timezone.utc).strftime("%a, %d %b %Y %H:%M:%S +0000")


def make_orig_tarball(out: Path, upstream: str) -> Path:
    """Create runeverything_<ver>.orig.tar.gz from repo (without debian/)."""
    name = f"{SOURCE}_{upstream}.orig.tar.gz"
    dest = out / name
    exclude_dirs = {".git", ".github", ".cursor", "dist", "bin", "debian"}
    exclude_files = {".env", ".DS_Store"}

    def filter_tar(ti: tarfile.TarInfo):
        parts = Path(ti.name).parts
        # strip top-level dir name check — we add as SOURCE-upstream/
        rel = parts[1:] if parts else parts
        if not rel:
            return ti
        if rel[0] in exclude_dirs:
            return None
        if rel[-1] in exclude_files:
            return None
        if rel[-1].endswith((".deb", ".rpm")):
            return None
        return ti

    prefix = f"{SOURCE}-{upstream}"
    with tarfile.open(dest, "w:gz") as tf:
        for path in sorted(ROOT.rglob("*")):
            if not path.is_file():
                continue
            rel = path.relative_to(ROOT)
            if rel.parts[0] in exclude_dirs:
                continue
            if path.name in exclude_files:
                continue
            if path.suffix in {".deb", ".rpm"}:
                continue
            ti = tarfile.TarInfo(name=f"{prefix}/{rel.as_posix()}")
            ti.size = path.stat().st_size
            ti.mtime = int(path.stat().st_mtime)
            ti.mode = 0o755 if path.stat().st_mode & 0o111 else 0o644
            with path.open("rb") as f:
                tf.addfile(ti, f)
    return dest


def make_debian_tarball(out: Path, debian_ver: str, series: str, upstream: str) -> Path:
    """Create .debian.tar.xz with series-specific changelog."""
    name = f"{SOURCE}_{debian_ver}.debian.tar.xz"
    dest = out / name
    with tempfile.TemporaryDirectory() as td:
        droot = Path(td) / "debian"
        shutil.copytree(ROOT / "debian", droot)
        # rewrite changelog distribution / version for this series
        rev = debian_ver.split("-", 1)[1]  # e.g. 1~noble1
        cl = (
            f"{SOURCE} ({debian_ver}) {series}; urgency=medium\n"
            f"\n"
            f"  * Launchpad PPA release for Ubuntu {series}.\n"
            f"\n"
            f" -- {MAINTAINER}  {rfc2822_now()}\n"
        )
        (droot / "changelog").write_text(cl)
        (droot / "rules").chmod(0o755)
        # xz compress via tarfile if possible; else subprocess
        try:
            with tarfile.open(dest, "w:xz") as tf:
                for path in sorted(droot.rglob("*")):
                    if path.is_dir():
                        continue
                    rel = path.relative_to(Path(td))
                    ti = tarfile.TarInfo(name=rel.as_posix())
                    ti.size = path.stat().st_size
                    ti.mtime = int(path.stat().st_mtime)
                    ti.mode = 0o755 if path.name == "rules" or (path.stat().st_mode & 0o111) else 0o644
                    with path.open("rb") as f:
                        tf.addfile(ti, f)
        except Exception:
            # fallback: tar | xz
            tar_path = Path(td) / "debian.tar"
            with tarfile.open(tar_path, "w") as tf:
                tf.add(droot, arcname="debian")
            subprocess.check_call(["xz", "-9", "-f", str(tar_path)])
            shutil.move(str(tar_path) + ".xz", dest)
    return dest


def parse_build_depends() -> str:
    control = (ROOT / "debian" / "control").read_text()
    m = re.search(r"Build-Depends:\s*((?:.|\n)*?)(?=\n[A-Z][\w-]+:|\n\n)", control)
    if not m:
        return "debhelper-compat (= 13)"
    deps = " ".join(line.strip() for line in m.group(1).splitlines())
    return re.sub(r"\s+", " ", deps).strip().rstrip(",")


def write_dsc(out: Path, debian_ver: str, orig: Path, deb_tar: Path) -> Path:
    dsc = out / f"{SOURCE}_{debian_ver}.dsc"
    files = [orig, deb_tar]
    body = io.StringIO()
    body.write(f"Format: 3.0 (quilt)\n")
    body.write(f"Source: {SOURCE}\n")
    body.write(f"Binary: {SOURCE}\n")
    body.write(f"Architecture: any\n")
    body.write(f"Version: {debian_ver}\n")
    body.write(f"Maintainer: {MAINTAINER}\n")
    body.write(f"Homepage: https://github.com/foqerhk/runeverything\n")
    body.write(f"Standards-Version: 4.6.2\n")
    body.write(f"Build-Depends: {parse_build_depends()}\n")
    body.write(f"Package-List:\n")
    body.write(f" {SOURCE} deb utils optional arch=any\n")
    body.write("Checksums-Sha1:\n")
    for f in files:
        body.write(f" {sha1(f)} {f.stat().st_size} {f.name}\n")
    body.write("Checksums-Sha256:\n")
    for f in files:
        body.write(f" {sha256(f)} {f.stat().st_size} {f.name}\n")
    body.write("Files:\n")
    for f in files:
        body.write(f" {md5(f)} {f.stat().st_size} {f.name}\n")
    dsc.write_text(body.getvalue())
    return dsc


def write_changes(
    out: Path,
    debian_ver: str,
    series: str,
    orig: Path,
    deb_tar: Path,
    dsc: Path,
) -> Path:
    changes = out / f"{SOURCE}_{debian_ver}_source.changes"
    files = [dsc, orig, deb_tar]
    # Files section uses md5; Checksums-Sha1 / Sha256 also required by modern Launchpad
    body = io.StringIO()
    body.write(f"Format: 1.8\n")
    body.write(f"Date: {rfc2822_now()}\n")
    body.write(f"Source: {SOURCE}\n")
    body.write(f"Binary: {SOURCE}\n")
    body.write(f"Architecture: source\n")
    body.write(f"Version: {debian_ver}\n")
    body.write(f"Distribution: {series}\n")
    body.write(f"Urgency: medium\n")
    body.write(f"Maintainer: {MAINTAINER}\n")
    body.write(f"Changed-By: {MAINTAINER}\n")
    body.write("Description:\n")
    body.write(f" {SOURCE} - Open-source remote agent for Intent Computing\n")
    body.write(f"Changes:\n")
    body.write(f" {SOURCE} ({debian_ver}) {series}; urgency=medium\n")
    body.write(f" .\n")
    body.write(f"   * Launchpad PPA release for Ubuntu {series}.\n")
    body.write(f"Checksums-Sha1:\n")
    for f in files:
        body.write(f" {sha1(f)} {f.stat().st_size} {f.name}\n")
    body.write(f"Checksums-Sha256:\n")
    for f in files:
        body.write(f" {sha256(f)} {f.stat().st_size} {f.name}\n")
    body.write(f"Files:\n")
    # Launchpad rejects section "-" ("Unknown section '-'").
    # Use the package Section/Priority from debian/control for every file.
    for f in files:
        body.write(f" {md5(f)} {f.stat().st_size} utils optional {f.name}\n")
    changes.write_text(body.getvalue())
    return changes


def clearsign_with_gpg(path: Path) -> None:
    """Clear-sign in place using GnuPG (preferred by Launchpad)."""
    gpg = shutil.which("gpg") or shutil.which("gpg2")
    if not gpg:
        raise FileNotFoundError("gpg not found")
    pw = GPG_DIR / "passphrase.txt"
    env = os.environ.copy()
    env["GNUPGHOME"] = env.get("GNUPGHOME") or str(Path.home() / ".gnupg")
    # Ensure key is present
    subprocess.run(
        [gpg, "--batch", "--import", str(GPG_DIR / "private.asc")],
        check=False,
        capture_output=True,
        env=env,
    )
    # Import public too so local verify works
    subprocess.run(
        [gpg, "--batch", "--import", str(GPG_DIR / "public.asc")],
        check=False,
        capture_output=True,
        env=env,
    )
    asc = Path(str(path) + ".asc")
    if asc.exists():
        asc.unlink()
    cmd = [
        gpg,
        "--batch",
        "--yes",
        "--pinentry-mode",
        "loopback",
        "--passphrase-file",
        str(pw),
        "--digest-algo",
        "SHA256",
        "--clearsign",
        "-o",
        str(asc),
        str(path),
    ]
    subprocess.run(cmd, check=True, env=env)
    asc.replace(path)


def clearsign_with_pgpy(path: Path, key: PGPKey) -> None:
    text = path.read_text()
    if not text.endswith("\n"):
        text += "\n"
    signed_msg = PGPMessage.new(text, cleartext=True)
    signed_msg |= key.sign(signed_msg, hash=HashAlgorithm.SHA256)
    path.write_text(str(signed_msg))


def clearsign(path: Path, key: PGPKey | None = None) -> None:
    # Prefer real GnuPG — Launchpad/soyuz often silently drops PGPy-signed uploads.
    # Fall back to PGPy when local gpg cannot unlock the key.
    if shutil.which("gpg") or shutil.which("gpg2"):
        try:
            clearsign_with_gpg(path)
            return
        except Exception as exc:
            print(f"==> gpg clearsign failed ({exc}); falling back to PGPy", file=sys.stderr)
    if key is None:
        raise RuntimeError("no usable gpg and no pgpy key")
    clearsign_with_pgpy(path, key)


def ftp_upload(files: list[Path]) -> None:
    # .changes must be last so Launchpad does not process a partial upload.
    ordered = sorted(files, key=lambda p: p.suffix == ".changes")
    incoming = f"~{PPA_USER}/{PPA_NAME}/ubuntu"
    print(f"==> FTP upload to ppa.launchpad.net:{incoming}/")
    ftp = FTP("ppa.launchpad.net", timeout=120)
    ftp.login()
    ftp.cwd(incoming)
    for f in ordered:
        print(f"  putting {f.name} ({f.stat().st_size} bytes)")
        with f.open("rb") as fh:
            ftp.storbinary(f"STOR {f.name}", fh)
    ftp.quit()
    print("==> upload complete; Launchpad will email build status to ruier09@qq.com")


def dput_upload(changes: Path) -> None:
    dput = shutil.which("dput")
    if not dput:
        raise FileNotFoundError("dput not found")
    # Remove stale .upload marker if any
    marker = changes.with_suffix(changes.suffix + ".ppa.upload")
    # dput creates e.g. foo_source.changes.ppa.upload
    for p in changes.parent.glob(changes.name + "*.upload"):
        p.unlink()
    subprocess.run(
        [dput, "-f", f"ppa:{PPA_USER}/{PPA_NAME}", str(changes)],
        check=True,
    )


def build_for_series(
    out: Path,
    upstream: str,
    series: str,
    orig: Path,
    dry_run: bool,
    revision: int,
) -> None:
    # Version scheme: 0.1.0-1~noble2 so each series/revision can coexist
    debian_ver = f"{upstream}-1~{series}{revision}"
    print(f"==> building {debian_ver} for {series}")
    deb_tar = make_debian_tarball(out, debian_ver, series, upstream)
    dsc = write_dsc(out, debian_ver, orig, deb_tar)

    key = None
    pw = (GPG_DIR / "passphrase.txt").read_text().strip()
    # Always load PGPy key as fallback; prefer gpg when it works.
    key, _ = PGPKey.from_file(str(GPG_DIR / "private.asc"))
    unlock_cm = key.unlock(pw)

    with unlock_cm:
        # IMPORTANT: sign .dsc BEFORE computing .changes checksums.
        clearsign(dsc, key)
        changes = write_changes(out, debian_ver, series, orig, deb_tar, dsc)
        clearsign(changes, key)

    # Sanity: .changes must reference current on-disk .dsc size
    cl = changes.read_text()
    dsc_size = str(dsc.stat().st_size)
    if dsc.name not in cl or dsc_size not in cl:
        raise RuntimeError(
            f"changes checksum size mismatch for {dsc.name}: "
            f"signed size={dsc_size}"
        )

    print(f"==> signed {dsc.name} ({dsc.stat().st_size} bytes) and {changes.name}")
    upload_files = [changes, dsc, orig, deb_tar]
    if dry_run:
        print("==> dry-run: skip upload")
        for f in upload_files:
            print(f"  {f} ({f.stat().st_size})")
        return
    if shutil.which("dput"):
        print("==> uploading via dput")
        dput_upload(changes)
    else:
        ftp_upload(upload_files)


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument("--version", default="0.1.0", help="upstream version")
    ap.add_argument(
        "--revision",
        type=int,
        default=1,
        help="debian revision suffix number, e.g. 2 -> 0.1.0-1~noble2",
    )
    ap.add_argument(
        "--series",
        action="append",
        dest="series",
        help="Ubuntu series (repeatable). Default: noble jammy",
    )
    ap.add_argument("--dry-run", action="store_true")
    ap.add_argument("--out", default="", help="output directory")
    ap.add_argument(
        "--reuse-orig",
        action="store_true",
        help="reuse existing dist/.../NAME_VERSION.orig.tar.gz (required after first Launchpad upload of that upstream version)",
    )
    ap.add_argument(
        "--orig",
        default="",
        help="path to an existing .orig.tar.gz to reuse",
    )
    args = ap.parse_args()
    series_list = args.series or ["noble", "jammy"]
    upstream = args.version
    out = Path(args.out) if args.out else ROOT / "dist" / f"ppa-v{upstream}"
    out.mkdir(parents=True, exist_ok=True)

    if not (GPG_DIR / "private.asc").is_file():
        print(f"missing {GPG_DIR}/private.asc", file=sys.stderr)
        return 1

    orig_name = f"{SOURCE}_{upstream}.orig.tar.gz"
    if args.orig:
        src = Path(args.orig).expanduser().resolve()
        if not src.is_file():
            print(f"missing --orig {src}", file=sys.stderr)
            return 1
        orig = out / orig_name
        if src.resolve() != orig.resolve():
            shutil.copy2(src, orig)
        print(f"==> reusing orig {orig} ({orig.stat().st_size} bytes)")
    elif args.reuse_orig and (out / orig_name).is_file():
        orig = out / orig_name
        print(f"==> reusing orig {orig} ({orig.stat().st_size} bytes)")
    else:
        print(f"==> orig tarball from {ROOT}")
        orig = make_orig_tarball(out, upstream)
        print(f"==> wrote {orig}")

    for series in series_list:
        build_for_series(out, upstream, series, orig, args.dry_run, args.revision)

    print()
    print(f"PPA: https://launchpad.net/~{PPA_USER}/+archive/ubuntu/{PPA_NAME}")
    print("Install once published:")
    print(f"  sudo add-apt-repository ppa:{PPA_USER}/{PPA_NAME}")
    print("  sudo apt update && sudo apt install runeverything")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
