"""Render a fixed Windows verification block from GitHub metadata. Never publishes."""
import argparse
import hashlib
import json
from pathlib import Path
import re
import subprocess

REPO = "Xpotato1024/xpotato-mcp-tunnel"
BASE = "https://github.com/" + REPO
TEMPLATE = Path(__file__).with_name("release-windows.ps1.in")


def positive_id(value):
    return type(value) is int and value > 0


def validate(release, version, expected_commit, tag_commit):
    if not re.fullmatch(r"\d+\.\d+\.\d+", version):
        raise ValueError("invalid version")
    if not re.fullmatch(r"[0-9a-f]{40}", expected_commit) or tag_commit != expected_commit:
        raise ValueError("tag must resolve to the adopted full source commit")
    tag = "v" + version
    if (not positive_id(release.get("id")) or release.get("tag_name") != tag or
            release.get("html_url") != BASE + "/releases/tag/" + tag or
            release.get("prerelease") is not False):
        raise ValueError("wrong release identity or prerelease")
    name = f"xpotato-tunnel-{version}-windows-amd64.zip"
    assets = [asset for asset in release.get("assets", []) if asset.get("name") == name]
    if len(assets) != 1:
        raise ValueError("Windows ZIP missing or ambiguous")
    asset = assets[0]
    digest = asset.get("digest")
    if (not positive_id(asset.get("id")) or not positive_id(asset.get("size")) or
            asset.get("state") != "uploaded" or not isinstance(digest, str) or
            not re.fullmatch(r"sha256:[0-9a-f]{64}", digest) or
            asset.get("browser_download_url") != BASE + "/releases/download/" + tag + "/" + name):
        raise ValueError("invalid asset identity, state, size or GitHub digest")
    return dict(name=name, digest=digest[7:], size=asset["size"], id=asset["id"])


def api(path):
    return json.loads(subprocess.check_output(
        ["gh", "api", "repos/" + REPO + "/" + path], text=True, encoding="utf-8"))


def fetch(version):
    if not re.fullmatch(r"\d+\.\d+\.\d+", version):
        raise ValueError("invalid version")
    release = api("releases/tags/v" + version)
    obj = api("git/ref/tags/v" + version)["object"]
    for _ in range(8):
        if obj.get("type") == "commit":
            return release, obj["sha"]
        if obj.get("type") != "tag" or not re.fullmatch(r"[0-9a-f]{40}", obj.get("sha", "")):
            raise ValueError("tag does not resolve to a commit")
        obj = api("git/tags/" + obj["sha"])["object"]
    raise ValueError("too many nested annotated tags")


def render(release, version, expected_commit, tag_commit):
    asset = validate(release, version, expected_commit, tag_commit)
    # Only validated numeric/hex/version constants enter executable PowerShell.
    block = TEMPLATE.read_text(encoding="utf-8")
    for key, value in dict(VERSION=version, COMMIT=expected_commit, SHA256=asset["digest"],
                           SIZE=asset["size"], RELEASE_ID=release["id"], ASSET_ID=asset["id"]).items():
        block = block.replace("@@" + key + "@@", str(value))
    return (f"## Windows ZIP の検証（v{version} 固定）\n\n"
            "下のブロック一回で固定版のZIP取得と検証を行います。公開リポジトリから認証なしで取得し、"
            "別のSHAファイル・Go・Node・npm・gh・トークンは不要です。\n\n"
            f"Source commit: `{expected_commit}` / platform: `windows-amd64` / dirty: `false`。\n\n"
            f"GitHub Release ID: `{release['id']}` / asset ID: `{asset['id']}` / "
            f"size: `{asset['size']}` / digest: `sha256:{asset['digest']}`。\n\n"
            "管理側が公開時にGitHub APIのdigestと実ZIPを照合し、下の値へ固定しています。"
            "実行時も固定tag/commit・Release/asset ID・GitHub側digestをAPIから取得して厳密照合します。\n\n"
            "下のブロック全体を通常ユーザーのPowerShellへ一回貼り付けます。"
            "展開先を変える場合は最後の引数だけ変更します。外側SHA照合後に、同じZIP bytesに含まれる"
            "検証スクリプトを実行します。exe・setup・updateは自動実行しません。\n\n"
            "```powershell\n" + block + "```\n\n"
            "`Verified` が出たら、版・source・platform・内部全SHAの検証完了です。"
            "版確認はbuild-info.jsonと検証済みexeのSHAに基づき、exeのversionコマンドは実行しません。"
            "失敗・取消時はsetup/updateへ進まず、取得元・保存先・エラーを確認してブロック全体を再実行します。"
            "既存の展開先は上書きせず全検証のみ行い、内容が違えば停止します。"
            "Windowsで検出・隔離があれば管理者へ検出内容を確認し、除外・保護無効化・隔離解除で回避しません。\n")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--version", required=True)
    parser.add_argument("--commit", required=True)
    parser.add_argument("--archive", required=True, type=Path)
    parser.add_argument("--output", required=True, type=Path)
    args = parser.parse_args()
    release, tag_commit = fetch(args.version)
    asset = validate(release, args.version, args.commit, tag_commit)
    if args.archive.name != asset["name"]:
        raise ValueError("wrong local archive filename")
    data = args.archive.read_bytes()
    if len(data) != asset["size"] or hashlib.sha256(data).hexdigest() != asset["digest"]:
        raise ValueError("local archive differs from GitHub digest/size")
    body = render(release, args.version, args.commit, tag_commit)
    with args.output.open("x", encoding="utf-8", newline="\n") as output:
        output.write(body)


if __name__ == "__main__":
    main()
