BookmarkHub for macOS - first launch
====================================

BookmarkHub is open-source but is not currently notarized with Apple.
macOS may block a ZIP downloaded from the internet even though the included
executables have an ad-hoc code signature.

Only continue if you downloaded BookmarkHub from a source you trust.

1. Extract the complete bookmarkhub-macos-*.zip file.
2. Open Terminal.
3. Type: sh followed by one space.
4. Drag macos-first-run.command from the extracted folder into Terminal.
5. Press Return.

The helper only restores executable permissions and removes the quarantine
attribute from bookmarkhub and versions/*/bookmarkhub-core inside this package.
It does not disable Gatekeeper or change global macOS security settings.

Package choice:
- Apple Silicon (M1/M2/M3/M4 and newer): bookmarkhub-macos-arm64.zip
- Intel Mac: bookmarkhub-macos-amd64.zip

After the first successful launch, run bookmarkhub normally from Terminal.
