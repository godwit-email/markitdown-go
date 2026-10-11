#!/bin/sh
set -eu
dest=${1:?usage: scripts/setup-python-oracle.sh DEST}
root=$(git rev-parse --show-toplevel)
commit=4cc9fa17653d695d64fb9eee5b33d4de55ff84e8
git -C "$root" cat-file -e "$commit^{commit}"
rm -rf "$dest/src"
mkdir -p "$dest/src"
git -C "$root" archive "$commit" packages/markitdown | tar -x -C "$dest/src"
srcfile=$dest/src/packages/markitdown/src/markitdown/_markitdown.py
grep -F 're.sub(r"\n{3,}", "\n\n", res.text_content)' "$srcfile" >/dev/null
python3 -m venv "$dest/venv"
"$dest/venv/bin/pip" install -q "$dest/src/packages/markitdown[pdf,docx,pptx,xlsx,xls]"
cat >"$dest/oracle" <<EOF
#!/bin/sh
exec "$dest/venv/bin/python" "$root/scripts/python_oracle.py" "\$@"
EOF
chmod +x "$dest/oracle"
