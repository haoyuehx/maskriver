# Quick Start

MaskRiver operates on authorised, isolated, disposable SQLite copies. It is not
an anonymisation or production-stable tool.

1. Build or install the linux/amd64 release; verify `sha256sum -c SHA256SUMS`.
2. Create separate source and target SQLite files (never point both at the same file).
3. Scan and preview:
   ```sh
   maskriver scan  --source source.db --target target.db --scope people.email ...
   maskriver mask  --source source.db --target target.db --scope people.email --key-file key.bin
   ```
4. Issue an approval in a separate process (see approval guide).
5. Apply only with the approval and explicit intent:
   ```sh
   maskriver mask --source ... --target ... --scope ... --key-file key.bin \
     --trust-file trust.json --history-dir history --approved --apply
   ```
6. Strict validation:
   ```sh
   maskriver validate --source ... --target ... --scope ... --key-file key.bin --strict
   ```

`--strict` requires a full untruncated scan and an adapter integrity check; it
fails closed if the adapter cannot provide one. See [limitations](../m3/platform-matrix.md).
