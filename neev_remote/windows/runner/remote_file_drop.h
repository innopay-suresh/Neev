#ifndef RUNNER_REMOTE_FILE_DROP_H_
#define RUNNER_REMOTE_FILE_DROP_H_

#include <windows.h>

#include <cstdint>
#include <string>
#include <vector>

// A single announced remote file (metadata only — no bytes yet).
struct RemoteFileEntry {
  std::wstring name;   // display name (no path separators)
  uint64_t size = 0;   // bytes
};

// Places a DELAYED-RENDER virtual-file group on the clipboard (COPY effect):
// the shell sees the file names/sizes immediately, but the CONTENTS are pulled
// only when the user pastes — at which point the data object calls back through
// FetchRemoteFileBytes() to obtain the bytes for one file. [token] identifies
// the announced set so the fetch callback can route to the right transfer.
// Must run in a process that owns the interactive clipboard (attended host).
// Returns true on OleSetClipboard success.
bool SetRemoteFileClipboard(const std::wstring& token,
                            const std::vector<RemoteFileEntry>& files);

// Implemented in clipboard_writer.cpp — the data object calls this (on the
// shell's paste thread) to obtain the bytes for file [index] of [token]. Blocks
// until the Dart side delivers them (it fetches over the peer connection) or the
// wait times out. Returns false on timeout/failure so paste fails cleanly rather
// than hanging forever.
bool FetchRemoteFileBytes(const std::wstring& token, uint32_t index,
                          std::vector<BYTE>& out);

#endif  // RUNNER_REMOTE_FILE_DROP_H_
