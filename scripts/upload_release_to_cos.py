#!/usr/bin/env python3
# /// script
# requires-python = ">=3.10"
# dependencies = [
#   "cos-python-sdk-v5>=1.9.36,<2",
# ]
# ///

import argparse
import os
import re
import tempfile
from pathlib import Path

from qcloud_cos import CosConfig, CosS3Client
from qcloud_cos.cos_exception import CosClientError, CosServiceError


TARGETS = (
    ("linux", "amd64", ".tar.gz"),
    ("linux", "arm64", ".tar.gz"),
    ("darwin", "amd64", ".tar.gz"),
    ("darwin", "arm64", ".tar.gz"),
    ("windows", "amd64", ".zip"),
)


def required_env(name: str, value: str | None) -> str:
    if not value:
        raise SystemExit(f"missing required value: {name}")
    return value


COS_PREFIX = "agr-cli"


def validate_inputs(tag: str, bucket: str, region: str) -> None:
    if not re.fullmatch(r"v[0-9]+\.[0-9]+\.[0-9]+(?:-[A-Za-z]+(?:\.[0-9]+)?)?", tag):
        raise SystemExit(f"invalid release tag: {tag}")
    if not re.fullmatch(r".+-[0-9]{5,}$", bucket):
        raise SystemExit("invalid COS bucket name, expected bucket-appid")
    if not re.fullmatch(r"[a-z]+(?:-[a-z0-9]+)+", region):
        raise SystemExit(f"invalid COS region: {region}")


def expected_artifacts(version: str) -> list[str]:
    names = [f"agr-{version}-{goos}-{goarch}{ext}" for goos, goarch, ext in TARGETS]
    names.extend(["checksums.txt", "install.sh"])
    return names


def validate_release_files(dist_dir: Path, installer: Path, tag: str) -> list[Path]:
    version = tag.removeprefix("v")
    if not dist_dir.is_dir():
        raise SystemExit(f"missing dist directory: {dist_dir}")
    if not installer.is_file():
        raise SystemExit(f"missing installer: {installer}")

    files = []
    for name in expected_artifacts(version):
        path = dist_dir / name
        if not path.is_file():
            raise SystemExit(f"missing release artifact: {path}")
        files.append(path)

    checksums = (dist_dir / "checksums.txt").read_text(encoding="utf-8")
    for name in expected_artifacts(version):
        if name in ("checksums.txt", "install.sh"):
            continue
        if not re.search(rf"\s{re.escape(name)}$", checksums, re.MULTILINE):
            raise SystemExit(f"missing checksum entry for: {name}")

    return files


def upload_file(client: CosS3Client, bucket: str, local_file: Path, key: str, dry_run: bool) -> None:
    print(f"upload {local_file} -> cos://<bucket>/{key}")
    if dry_run:
        return
    try:
        client.upload_file(
            Bucket=bucket,
            LocalFilePath=str(local_file),
            Key=key,
            EnableMD5=True,
        )
    except CosServiceError as exc:
        details = [
            f"status={exc.get_status_code()}",
            f"code={exc.get_error_code()}",
            f"request_id={exc.get_request_id()}",
        ]
        message = exc.get_error_msg()
        if message:
            details.append(f"message={sanitize_message(message)}")
        raise SystemExit(f"failed to upload {key}: {', '.join(details)}") from None
    except CosClientError as exc:
        raise SystemExit(f"failed to upload {key}: {sanitize_message(str(exc))}") from None


def sanitize_message(message: str) -> str:
    replacements = {
        os.getenv("COS_RELEASE_BUCKET", ""): "<bucket>",
        os.getenv("COS_RELEASE_SECRET_ID", ""): "<secret-id>",
        os.getenv("COS_RELEASE_SECRET_KEY", ""): "<secret-key>",
    }
    sanitized = message
    for value, label in replacements.items():
        if value:
            sanitized = sanitized.replace(value, label)
    return sanitized


def main() -> int:
    parser = argparse.ArgumentParser(description="Upload AGR CLI release artifacts to Tencent Cloud COS.")
    parser.add_argument("--tag", default=os.getenv("RELEASE_TAG"), required=False)
    parser.add_argument("--dist-dir", default="dist")
    parser.add_argument("--installer", default="install.sh")
    parser.add_argument("--bucket", default=os.getenv("COS_RELEASE_BUCKET"), required=False)
    parser.add_argument("--region", default=os.getenv("COS_RELEASE_REGION"), required=False)
    parser.add_argument("--dry-run", action="store_true")
    args = parser.parse_args()

    tag = required_env("RELEASE_TAG or --tag", args.tag)
    bucket = required_env("COS_RELEASE_BUCKET or --bucket", args.bucket)
    region = required_env("COS_RELEASE_REGION or --region", args.region)
    secret_id = required_env("COS_RELEASE_SECRET_ID", os.getenv("COS_RELEASE_SECRET_ID"))
    secret_key = required_env("COS_RELEASE_SECRET_KEY", os.getenv("COS_RELEASE_SECRET_KEY"))
    validate_inputs(tag, bucket, region)

    dist_dir = Path(args.dist_dir)
    installer = Path(args.installer)
    artifact_files = validate_release_files(dist_dir, installer, tag)

    client = CosS3Client(
        CosConfig(
            Region=region,
            SecretId=secret_id,
            SecretKey=secret_key,
            Scheme="https",
        )
    )

    for local_file in artifact_files:
        upload_file(
            client,
            bucket,
            local_file,
            f"{COS_PREFIX}/{tag}/{local_file.name}",
            args.dry_run,
        )

    upload_file(client, bucket, installer, f"{COS_PREFIX}/latest/install.sh", args.dry_run)

    with tempfile.TemporaryDirectory() as tmp:
        version_file = Path(tmp) / "VERSION"
        version_file.write_text(f"{tag}\n", encoding="utf-8")
        upload_file(client, bucket, version_file, f"{COS_PREFIX}/latest/VERSION", args.dry_run)

    print("COS release upload complete.")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
