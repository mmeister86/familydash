#!/usr/bin/env bash
# Builds the bridge as a single binary with an embedded Info.plist, so macOS
# can show the Reminders permission prompt even when it runs under launchd.
set -euo pipefail
cd "$(dirname "$0")"

swiftc -O reminders-bridge.swift -o familydash-reminders \
  -Xlinker -sectcreate -Xlinker __TEXT -Xlinker __info_plist -Xlinker Info.plist

codesign --force --sign - --identifier de.matthiasmeister.familydash.reminders familydash-reminders

echo "✓ built ./familydash-reminders"
echo "  Erster Start im Terminal (für den Berechtigungsdialog):"
echo "  DASH_URL=http://192.168.188.127:8080/api/reminders DASH_TOKEN=… ./familydash-reminders"
