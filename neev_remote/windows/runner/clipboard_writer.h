#ifndef RUNNER_CLIPBOARD_WRITER_H_
#define RUNNER_CLIPBOARD_WRITER_H_

#include <flutter/flutter_engine.h>

// Registers the "neev_remote/clipboard" MethodChannel. Its sole method,
// "writeFilesCopy", puts a CF_HDROP file list on the clipboard together with a
// "Preferred DropEffect" of DROPEFFECT_COPY, so a subsequent Ctrl+V in Explorer
// COPIES the files. The `pasteboard` package writes CF_HDROP without that
// format, and Windows then defaults the paste to a MOVE — which deleted the
// mirrored file after paste. Runs in the app's own (interactive) session, so it
// works for an attended host; a SYSTEM host uses the helper's clip agent.
void RegisterClipboardWriter(flutter::FlutterEngine* engine);

#endif  // RUNNER_CLIPBOARD_WRITER_H_
