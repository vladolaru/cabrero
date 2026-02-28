// cabrero-notify — lightweight macOS notification helper.
//
// Sends a macOS notification without loading the AppleScript runtime.
// This avoids the scripting additions (Standard Additions, Digital Hub)
// that pull in CoreAudio/AudioToolbox frameworks and trigger macOS TCC
// prompts for Desktop, Music Library, and Photos.
//
// Usage: cabrero-notify <title> <message>
//
// Uses NSUserNotification (deprecated since macOS 11 but still functional).
// If Apple removes this API, switch to UNUserNotificationCenter with an
// app bundle (requires one-time notification authorization).

import Cocoa

guard CommandLine.arguments.count >= 3 else {
    fputs("usage: cabrero-notify <title> <message>\n", stderr)
    exit(1)
}

let title = CommandLine.arguments[1]
let body = CommandLine.arguments[2]

let notification = NSUserNotification()
notification.title = title
notification.informativeText = body

NSUserNotificationCenter.default.deliver(notification)
