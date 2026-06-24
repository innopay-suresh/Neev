# Neev Remote - Flutter Desktop Specification

## 1. Project Overview

**Project Name:** Neev Remote
**Type:** Cross-platform Remote Desktop Application (Desktop Client + Agent)

**Core Functionality:** A unified desktop application that works as both a remote viewer (client) and a remote agent (host), enabling cross-platform screen sharing and remote control with bundled dependencies.

**Target Platforms:**
- Windows 10/11 (x64)
- macOS 12+ (Intel & Apple Silicon)
- Linux (Ubuntu 22.04+, Fedora 38+)

---

## 2. UI/UX Specification

### 2.1 Window Structure

**Main Window:**
- Single-window application with tab-based navigation
- Minimum size: 800x600
- Default size: 1200x800
- Native window controls (close, minimize, maximize)

**Dialogs:**
- Connection dialog (modal)
- Settings dialog (modal)
- About dialog (modal)

### 2.2 Visual Design

**Color Palette:**
```
Primary:        #2563EB (Blue 600)
Primary Dark:   #1D4ED8 (Blue 700)
Secondary:      #64748B (Slate 500)
Background:     #0F172A (Slate 900)
Surface:        #1E293B (Slate 800)
Surface Light:  #334155 (Slate 700)
Text Primary:   #F8FAFC (Slate 50)
Text Secondary: #94A3B8 (Slate 400)
Success:        #22C55E (Green 500)
Warning:        #F59E0B (Amber 500)
Error:          #EF4444 (Red 500)
```

**Typography:**
```
Font Family: System default (Segoe UI on Windows, SF Pro on Mac, Ubuntu on Linux)
Heading 1: 24px, Bold
Heading 2: 20px, SemiBold
Body: 14px, Regular
Caption: 12px, Regular
```

**Spacing System:**
- Base unit: 4px
- Margins: 16px (standard), 24px (large sections)
- Padding: 8px (small), 12px (medium), 16px (large)
- Border radius: 8px (cards), 4px (buttons)

### 2.3 Layout Structure

```
┌─────────────────────────────────────────────────────────┐
│ [Native Title Bar]                          [─] [□] [×] │
├─────────────────────────────────────────────────────────┤
│ ┌─────────┐                                             │
│ │ [Logo]  │  Neev Remote         [Agent Status] [⚙️]   │
│ └─────────┘                                             │
├──────────┬──────────────────────────────────────────────┤
│          │                                              │
│  Sidebar │              Main Content Area               │
│          │                                              │
│  ━━━━━━━ │  ┌────────────────────────────────────────┐  │
│  Agent   │  │                                        │  │
│  ━━━━━━━ │  │           Remote Screen View          │  │
│  Viewer  │  │                                        │  │
│  ━━━━━━━ │  │                                        │  │
│  Settings│  │                                        │  │
│          │  └────────────────────────────────────────┘  │
│          │                                              │
│          │  [Quality] [FPS] [Latency]     [Toolbar]    │
├──────────┴──────────────────────────────────────────────┤
│ Status: Connected to DESKTOP-PC (192.168.1.100)  ●      │
└─────────────────────────────────────────────────────────┘
```

### 2.4 Screens

**1. Home/Agent Screen**
- Agent ID display with copy button
- Connection status indicator
- QR code for easy connection
- "Start/Stop Agent" toggle
- Recent connections list

**2. Viewer/Connect Screen**
- Agent ID / URL input field
- Connect button
- Recent connections grid
- Connection history

**3. Active Session Screen**
- Full remote screen display
- Floating toolbar (hidden by default, shown on mouse move)
- Quality indicators
- Input mode toggle (control/view only)

**4. Settings Screen**
- Tabs: General, Connection, Video, Audio, Shortcuts, About
- Video quality presets (Balanced, Performance, Quality)
- Codec selection (H.264 primary, VP8 fallback)
- Auto-answer toggle
- Start on boot toggle

---

## 3. Functionality Specification

### 3.1 Core Features

**Agent Mode:**
- Start/stop agent service
- Display unique Agent ID (format: XXX-XXX-XXX)
- QR code generation for mobile scanning
- Connection permission dialog
- Multi-viewer support (up to 5 concurrent)
- Wake-on-LAN support

**Viewer Mode:**
- Connect via Agent ID
- Connect via URL/link
- Connection history
- Bandwidth auto-detection

**Remote Control:**
- Full mouse control
- Keyboard input (including special keys)
- Clipboard sync (text and files)
- Multi-monitor support
- High-DPI scaling

**Video Encoding:**
- H.264 (primary) via platform APIs:
  - Windows: Media Foundation
  - macOS: VideoToolbox
  - Linux: VAAPI
- Software fallback: libx264 (bundled)
- Adaptive bitrate (ABR)
- Keyframe control

**Audio (Future):**
- System audio streaming
- Microphone passthrough

### 3.2 Data Flow

```
┌─────────────────────────────────────────────────────────┐
│                    SIGNALING SERVER                     │
│              (WebSocket Relay / STUN/TURN)              │
└──────────┬──────────────────────────────────┬───────────┘
           │                                  │
    ┌──────▼──────┐                   ┌──────▼──────┐
    │  Agent App  │                   │ Viewer App  │
    │             │◄──── WebRTC ────►│             │
    │ ┌─────────┐ │                   │ ┌─────────┐ │
    │ │Capture  │ │                   │ │Display  │ │
    │ │(Screen) │ │                   │ │(Video)  │ │
    │ └────┬────┘ │                   │ └────▲────┘ │
    │      │      │                   │      │      │
    │ ┌────▼────┐ │                   │ ┌────┴────┐ │
    │ │ H.264   │ │                   │ │ H.264   │ │
    │ │ Encoder │ │                   │ │ Decoder │ │
    │ └─────────┘ │                   │ └─────────┘ │
    └─────────────┘                   └─────────────┘
```

### 3.3 WebRTC Configuration

```dart
// STUN servers
stun:stun.l.google.com:19302
stun:stun1.l.google.com:19302

// TURN server (for NAT traversal)
turn:turnserver:3478

// Video codec priority
1. H.264 Constrained Baseline (main profile on Windows)
2. VP8 (fallback)

// Video settings
- Resolution: Adaptive (up to 1920x1080)
- FPS: 30 (adaptive down to 5)
- Bitrate: 1500kbps (adaptive 200kbps - 5000kbps)
- Keyframe interval: 2 seconds
```

### 3.4 Error Handling

| Error | User Message | Recovery Action |
|-------|-------------|-----------------|
| Connection timeout | "Connection timed out. Check agent ID and try again." | Show retry button |
| Agent offline | "Agent is offline. Start the agent on remote computer." | Remove from recent |
| ICE failed | "Could not establish connection. Try again." | Retry with TURN fallback |
| Encoding failed | "Video encoding error. Restarting..." | Auto-retry |
| Network lost | "Connection lost. Reconnecting..." | Auto-reconnect |

---

## 4. Technical Specification

### 4.1 Technology Stack

```yaml
Framework: Flutter 3.38+
Dart: 3.10+
State Management: Riverpod 2.0+
Architecture: Clean Architecture (Presentation / Domain / Data)

Key Packages:
  - flutter_webrtc: ^4.0.0    # WebRTC peer connection
  - desktop_lifecycle: ^0.1.0 # Window management
  - hotkey_manager: ^0.2.0    # Global shortcuts
  - window_manager: ^0.4.0    # Window controls
  - system_tray: ^2.0.0       # System tray
  - ffmpeg_kit_flutter: ^6.0  # Video encoding (bundled)
  - go_router: ^14.0          # Navigation
  - shared_preferences: ^2.0  # Settings storage
```

### 4.2 Platform Channels

**macOS:**
- Screen capture: CGDisplayStream (Cursor baked in)
- Audio: CoreAudio

**Windows:**
- Screen capture: DirectX Graphics Capture
- Audio: WASAPI

**Linux:**
- Screen capture: PipeWire / X11 (fallback)
- Audio: PulseAudio / PipeWire

### 4.3 Video Encoding Pipeline

```dart
// Capture → RGBA Frame → H.264 Encode → RTP Packetize → WebRTC Track → Network
//                      ↓
//              Adaptive Bitrate
//              (based on network stats)
```

### 4.4 File Structure

```
lib/
├── main.dart
├── app.dart
├── core/
│   ├── constants/
│   ├── errors/
│   ├── theme/
│   └── utils/
├── data/
│   ├── models/
│   ├── repositories/
│   └── services/
├── domain/
│   ├── entities/
│   ├── repositories/
│   └── usecases/
└── presentation/
    ├── pages/
    ├── widgets/
    └── providers/
```

### 4.5 Dependencies (Bundled)

| Dependency | Windows | macOS | Linux |
|------------|---------|-------|-------|
| FFmpeg | ✅ | ✅ | ✅ |
| OpenSSL | ✅ | ✅ | ✅ |
| WebView2 | ✅ (bundled) | N/A | N/A |
| GTK | N/A | N/A | ✅ |

### 4.6 Build Targets

```bash
# Windows
flutter build windows --release --target-platform windows-x64

# macOS  
flutter build macos --release --target-platform macos-x64,macos-arm64

# Linux
flutter build linux --release --target-platform linux-x64
```

---

## 5. Implementation Phases

### Phase 1: Core Setup
- Flutter project setup
- Clean architecture structure
- Theme and design system
- Basic navigation

### Phase 2: WebRTC Integration
- Signaling client
- Peer connection
- Video track handling
- Data channels (input, clipboard)

### Phase 3: Agent Features
- Screen capture
- Video encoding
- Agent registration
- Connection handling

### Phase 4: Viewer Features
- Remote display
- Input injection
- Clipboard sync
- Connection UI

### Phase 5: Polish
- System tray
- Global hotkeys
- Auto-start
- Code signing

---

## 6. Success Criteria

- [ ] App builds successfully on Windows, macOS, Linux
- [ ] Mac→Windows remote control works
- [ ] Windows→Mac remote control works  
- [ ] Mac→Mac remote control works
- [ ] Windows→Windows remote control works
- [ ] Linux→Windows remote control works
- [ ] Package size under 200MB per platform
- [ ] App runs without additional dependencies
- [ ] Connection establishes within 10 seconds
- [ ] Video latency under 200ms on LAN