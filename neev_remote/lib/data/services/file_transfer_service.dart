import 'dart:convert';
import 'dart:typed_data';

import 'package:uuid/uuid.dart';

import 'file_store.dart';

enum FileDirection { incoming, outgoing }

// active = bytes moving; sent = all bytes delivered to the channel but the host
// has NOT yet confirmed a distinct file was saved (never show this as success);
// done = host confirmed the file is fully + uniquely written (or, for an incoming
// transfer, we wrote it); error = failed/cancelled.
enum FileStatus { active, sent, done, error }

/// One file transfer, either being sent to or received from the remote peer.
class FileTransfer {
  FileTransfer({
    required this.id,
    required this.name,
    required this.size,
    required this.direction,
    this.transferred = 0,
    this.status = FileStatus.active,
    this.savedPath,
    this.error,
    this.clipboard = false,
  });

  final String id;
  final String name;
  final int size; // total bytes; 0 if unknown
  final FileDirection direction;
  int transferred; // bytes moved so far
  FileStatus status;
  String? savedPath; // where an incoming file landed
  String? error;
  // True when this transfer is a clipboard mirror (copy→paste): the receiver
  // stages it to temp and puts it on the OS clipboard instead of Downloads.
  bool clipboard;

  double get progress =>
      size > 0 ? (transferred / size).clamp(0.0, 1.0).toDouble() : 0.0;
}

class _Incoming {
  _Incoming(this.ft) : buf = BytesBuilder(copy: false);
  final FileTransfer ft;
  final BytesBuilder buf;
  // Unique destination path, reserved (atomically created) the MOMENT the offer
  // arrives — keyed off this transfer, never a shared/reused path. Because the
  // placeholder exists on disk before the next offer is handled, two rapid
  // transfers can never resolve to the same name and clobber each other.
  Future<String>? reserved;
}

/// Chunked file transfer over a text data channel. Wire messages (all JSON):
///   {k:'ft', t:'offer', id, name, size}   announce a transfer
///   {k:'ft', t:'data',  id, seq, d}       base64 chunk (ordered)
///   {k:'ft', t:'end',   id}               sender finished pushing bytes
///   {k:'ft', t:'saved', id, path}         RECEIVER→sender: file fully + uniquely
///                                         written (the only thing that turns a
///                                         send from "Delivered" into confirmed)
///   {k:'ft', t:'cancel',id}               aborted
/// Phase 1 buffers each transfer in memory (see [maxFile]); streaming to disk
/// is a later refinement.
class FileTransferManager {
  FileTransferManager({
    required this.send,
    required this.buffered,
    required this.store,
    required this.onChange,
    this.onRequest,
    this.onClipboardFile,
  });

  /// Sends one JSON message on the peer's file channel.
  final void Function(String json) send;

  /// Current bytes queued on the file channel (for send pacing); 0 if unknown.
  final int Function() buffered;

  final FileStore store;
  final void Function() onChange;

  /// Called when the peer asks us to share a file (their "Import") — should open
  /// a picker and send the chosen file back.
  final Future<void> Function()? onRequest;

  /// Called when a *clipboard* file finishes arriving, with the staged temp
  /// path — the owner puts it on the OS clipboard so Ctrl+V pastes the file.
  final Future<void> Function(String stagedPath)? onClipboardFile;

  /// Raw bytes per 'data' message; base64 inflates 36 KB → 48 KB, well under
  /// the ~256 KB channel limit.
  static const int rawChunk = 36 * 1024;

  /// Pause sending while more than this many bytes are queued in the channel's
  /// SCTP send buffer. Kept SMALL (512 KB) on purpose: libwebrtc's send buffer
  /// caps around 16 MB, and if we let it fill (as the old 4 MB high-water + a
  /// give-up-and-send-anyway loop did) it saturates after ~4 medium files and
  /// wedges BOTH directions of the one shared bidirectional file channel until
  /// reconnect. Draining to 512 KB between chunks keeps it nowhere near the cap.
  static const int _highWater = 512 * 1024;

  /// If the buffer won't drain for this long the peer isn't receiving — abort
  /// the transfer as failed. NEVER force-send into a full buffer (that's what
  /// corrupted the channel and broke every later transfer).
  static const Duration _drainTimeout = Duration(seconds: 30);

  /// In-memory size cap. Raised to 2 GB (was 200 MB, which silently rejected
  /// real installers — .dmg/.pkg/.exe — so they "failed both ways"). Matches the
  /// clipboard-file cap. Chunks are base64'd per-slice on send, so only the raw
  /// bytes sit in memory; multi-GB streaming-to-disk is a later refinement.
  static const int maxFile = 2 * 1024 * 1024 * 1024;

  final _uuid = const Uuid();
  final List<FileTransfer> transfers = [];
  final Map<String, _Incoming> _incoming = {};

  /// Send [bytes] as [name] to the peer. Returns the transfer, or null if the
  /// file is too large.
  Future<FileTransfer?> sendFile(String name, Uint8List bytes,
      {bool clipboard = false}) async {
    if (bytes.length > maxFile) return null;
    final id = _uuid.v4();
    final t = FileTransfer(
        id: id,
        name: name,
        size: bytes.length,
        direction: FileDirection.outgoing,
        clipboard: clipboard);
    transfers.insert(0, t);
    onChange();

    send(jsonEncode({
      'k': 'ft',
      't': 'offer',
      'id': id,
      'name': name,
      'size': bytes.length,
      if (clipboard) 'clip': 1,
    }));

    var seq = 0;
    for (var off = 0; off < bytes.length; off += rawChunk) {
      if (t.status == FileStatus.error) return t; // cancelled
      final end = off + rawChunk < bytes.length ? off + rawChunk : bytes.length;
      // Encode just this slice so we never build a huge base64 string.
      final chunk = base64Encode(bytes.sublist(off, end));
      send(jsonEncode(
          {'k': 'ft', 't': 'data', 'id': id, 'seq': seq, 'd': chunk}));
      seq++;
      t.transferred = end;
      onChange();
      // Wait for the SCTP buffer to DRAIN before queuing more — the bufferedAmount
      // is updated by native events, so this poll sees real values. If it won't
      // drain within the timeout, the peer has stopped receiving: abort THIS
      // transfer (never force-send into a full buffer). The channel stays healthy
      // so the next transfer — and the other direction — still work.
      await Future<void>.delayed(Duration.zero);
      var waited = 0;
      while (buffered() > _highWater) {
        if (t.status == FileStatus.error) return t; // cancelled
        await Future<void>.delayed(const Duration(milliseconds: 15));
        waited += 15;
        if (waited > _drainTimeout.inMilliseconds) {
          t.status = FileStatus.error;
          t.error = 'Transfer stalled — the other side stopped receiving.';
          onChange();
          return t;
        }
      }
    }
    if (t.status == FileStatus.error) return t;
    send(jsonEncode({'k': 'ft', 't': 'end', 'id': id}));
    t.transferred = t.size;
    // NOT "done" — bytes are on the channel, but the host hasn't confirmed a
    // distinct file was saved yet. Only a {t:'saved'} ack (handled below) flips
    // this to done. If the host never acks (e.g. an older build), it stays
    // "Delivered — confirming…", which is honest, not a false "Sent".
    t.status = FileStatus.sent;
    onChange();
    return t;
  }

  /// Handle an inbound {k:'ft', ...} message.
  void handleMessage(Map<String, dynamic> m) {
    final t = m['t'] as String?;
    if (t == 'request') {
      onRequest?.call();
      return;
    }
    final id = m['id'] as String?;
    if (id == null) return;
    switch (t) {
      case 'offer':
        final name = (m['name'] as String?) ?? 'file';
        final size = (m['size'] as int?) ?? 0;
        final ft = FileTransfer(
            id: id,
            name: name,
            size: size,
            direction: FileDirection.incoming,
            clipboard: m['clip'] == 1);
        if (size > maxFile) {
          ft.status = FileStatus.error;
          ft.error = 'File exceeds the ${maxFile ~/ (1024 * 1024)} MB limit';
          transfers.insert(0, ft);
          onChange();
          return;
        }
        transfers.insert(0, ft);
        final inc = _Incoming(ft);
        // Reserve a UNIQUE destination now, at offer time (not at 'end'), so the
        // placeholder is on disk before any later transfer picks a name. This is
        // what makes rapid back-to-back sends land as separate files instead of
        // overwriting one shared slot.
        if (store.supported) inc.reserved = store.reserveUnique(name);
        _incoming[id] = inc;
        onChange();
        break;
      case 'data':
        final inc = _incoming[id];
        final d = m['d'] as String?;
        if (inc == null || d == null) return;
        final bytes = base64Decode(d);
        inc.buf.add(bytes);
        inc.ft.transferred += bytes.length;
        onChange();
        break;
      case 'end':
        final inc = _incoming.remove(id);
        if (inc != null) _finishIncoming(inc);
        break;
      case 'saved':
        // RECEIVER confirmed a distinct file was fully written. Flip the matching
        // OUTGOING transfer from "sent" (delivered) to done (confirmed). This is
        // the ONLY place a send is allowed to show success.
        for (final tr in transfers) {
          if (tr.id == id && tr.direction == FileDirection.outgoing) {
            tr.savedPath = m['path'] as String?;
            tr.status = FileStatus.done;
            onChange();
            break;
          }
        }
        break;
      case 'cancel':
        final inc = _incoming.remove(id);
        if (inc != null) {
          inc.ft.status = FileStatus.error;
          inc.ft.error = 'Cancelled by sender';
          // Drop the empty placeholder we reserved at offer time.
          if (store.supported) {
            inc.reserved?.then((p) => store.deleteQuietly(p)).catchError((_) {});
          }
          onChange();
        }
        break;
    }
  }

  Future<void> _finishIncoming(_Incoming inc) async {
    try {
      final bytes = inc.buf.takeBytes();
      String? path;
      if (store.supported) {
        // Write to the destination reserved at offer time (unique per transfer).
        // Fall back to reserving now only if the offer path somehow wasn't set.
        path = await (inc.reserved ?? store.reserveUnique(inc.ft.name));
        await store.writeReserved(path, bytes);
        // Clipboard mirror also lands in Downloads (always findable) AND on the
        // OS clipboard for Ctrl+V — CF_HDROP paste is fragile across the
        // SYSTEM / cross-user boundary, so the saved file is the reliable path.
        if (inc.ft.clipboard) await onClipboardFile?.call(path);
      }
      inc.ft.savedPath = path;
      inc.ft.transferred = inc.ft.size == 0 ? bytes.length : inc.ft.size;
      inc.ft.status = FileStatus.done;
      // Tell the sender the file is fully + uniquely saved, so its status can go
      // from "Delivered" to confirmed. Without this the sender can only ever
      // guess — which is how 4 overwrites previously showed as 5 "Sent".
      if (path != null) {
        send(jsonEncode(
            {'k': 'ft', 't': 'saved', 'id': inc.ft.id, 'path': path}));
      }
    } catch (e) {
      inc.ft.status = FileStatus.error;
      inc.ft.error = e.toString();
    }
    onChange();
  }

  void clearFinished() {
    // Keep "active" AND "sent" (delivered-but-unconfirmed) rows — only remove
    // truly finished ones (host-confirmed done, or failed).
    transfers.removeWhere(
        (t) => t.status == FileStatus.done || t.status == FileStatus.error);
    onChange();
  }
}
