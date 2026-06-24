/**
 * useWebRTC — manages the WebRTC PeerConnection for the browser controller.
 *
 * Flow:
 *  1. Agent answers the controller's connect request (signaling).
 *  2. Controller calls start() → creates offer → sends via signaling.
 *  3. Agent responds with answer + ICE candidates.
 *  4. OnTrack → stream set → rendered in <video>.
 *  5. DataChannel → sendInput() sends JSON events to the agent.
 */

import { useCallback, useRef, useState } from 'react'

const ICE_SERVERS = [
  { urls: 'stun:stun.l.google.com:19302' },
  { urls: 'stun:stun1.l.google.com:19302' },
]

export function useWebRTC({ send, on, peerID, iceServers = ICE_SERVERS, getLocalIPs }) {
  const pcRef    = useRef(null)
  const dcRef    = useRef(null)
  const clipRef  = useRef(null)
  const chatRef  = useRef(null)
  const fileRef  = useRef(null)
  const [stream, setStream]           = useState(null)
  const [connectionState, setState]   = useState('new')
  const [displays, setDisplays]       = useState([])
  const [cursorInfo, setCursorInfo]   = useState({ x: 0, y: 0, visible: true })
  const [connectionMode, setConnectionMode] = useState('connecting')
  const [qualityInfo, setQualityInfo] = useState({ fps: 0, rtt: 0, loss: 0, bw: 0 })
  const [wolEnabled, setWolEnabled] = useState(false)
  const cleanups = useRef([])

  const cleanup = useCallback(() => {
    cleanups.current.forEach(fn => fn())
    cleanups.current = []
    pcRef.current?.close()
    pcRef.current = null
    dcRef.current = null
    clipRef.current = null
    chatRef.current = null
    fileRef.current = null
    setStream(null)
    setDisplays([])
    setState('closed')
    if (typeof window !== 'undefined') {
      window.__remoteAgentWebRTC = { status: 'closed', streamTracks: 0 }
    }
  }, [])

  const start = useCallback(async (targetID) => {
    const logViewer = (level, message) => {
      if (typeof window !== 'undefined') {
        window.dispatchEvent(new CustomEvent('webrtc:log_received', {
          detail: {
            time: new Date().toLocaleTimeString(),
            source: 'Viewer',
            level,
            message
          }
        }))
      }
    }

    logViewer('info', `Initiating signaling connect to agent ${targetID}...`)

    // Clean up any existing connection.
    cleanup();
    // Create PeerConnection.
    const pc = new RTCPeerConnection({ iceServers: iceServers.length ? iceServers : ICE_SERVERS })
    pcRef.current = pc

    pc.onconnectionstatechange = () => {
      console.log('[webrtc] connectionState', pc.connectionState)
      logViewer('info', `WebRTC connection state: ${pc.connectionState}`)
      setState(pc.connectionState)
      if (typeof window !== 'undefined') {
        window.__remoteAgentWebRTC = {
          ...(window.__remoteAgentWebRTC || {}),
          connectionState: pc.connectionState,
        }
      }
    }

    pc.oniceconnectionstatechange = () => {
      console.log('[webrtc] iceConnectionState', pc.iceConnectionState)
      if (typeof window !== 'undefined') {
        window.__remoteAgentWebRTC = {
          ...(window.__remoteAgentWebRTC || {}),
          iceConnectionState: pc.iceConnectionState,
        }
      }
    }

    // Receive remote video track.
    pc.ontrack = (evt) => {
      console.log('[webrtc] ontrack', {
        streams: evt.streams?.length,
        trackId: evt.track?.id,
        trackKind: evt.track?.kind,
        codec: evt.track?.getSettings?.()?.codec,
      })
      const remoteStream = evt.streams?.[0] || new MediaStream([evt.track])
      console.log('[webrtc] remoteStream', { id: remoteStream.id, trackCount: remoteStream.getTracks().length })
      logViewer('info', `Received remote video track: ${evt.track?.kind}, codec: ${evt.track?.getSettings?.()?.codec}`)
      setStream(remoteStream)
      if (typeof window !== 'undefined') {
        window.__remoteAgentWebRTC = {
          ...(window.__remoteAgentWebRTC || {}),
          streamTracks: remoteStream.getTracks().length,
          streamId: remoteStream.id,
        }
      }
    }

    // Forward local ICE candidates to agent.
    pc.onicecandidate = (evt) => {
      if (evt.candidate) {
        console.log('[webrtc] local ICE candidate', evt.candidate.candidate)
        logViewer('info', `ICE candidate gathered: ${evt.candidate.candidate.split(' ')[0]}`)
        
        send({ type: 'candidate', to: targetID, payload: evt.candidate.toJSON() })

        // Workaround for macOS WebKit blocking local IPs and exposing only mDNS `.local` addresses.
        // We inject actual local IPs to ensure the Windows Agent can initiate outbound STUN.
        if (evt.candidate.candidate.includes('.local') && getLocalIPs) {
          getLocalIPs().then(ips => {
            ips.forEach(ip => {
              const newCandStr = evt.candidate.candidate.replace(/[a-zA-Z0-9-]+\.local/, ip)
              const newCand = { ...evt.candidate.toJSON(), candidate: newCandStr }
              send({ type: 'candidate', to: targetID, payload: newCand })
              logViewer('info', `Injected local IP candidate: ${ip}`)
            })
          }).catch(err => console.warn('Failed to get local IPs', err))
        }
      }
    }

    // Request a receive-only video transceiver so the controller will accept the agent video track.
    pc.addTransceiver('video', { direction: 'recvonly' })

    // Queue for early remote candidates
    const pendingCandidates = []

    // Subscribe to signaling messages before sending the offer to avoid race conditions.
    const unsubAnswer = on('answer', async (msg) => {
      if (msg.from !== targetID) return
      console.log('[webrtc] received answer', msg)
      logViewer('info', 'Received SDP answer from agent')
      if (pc.signalingState === 'have-local-offer' || pc.signalingState === 'have-remote-pranswer') {
        await pc.setRemoteDescription(new RTCSessionDescription(msg.payload))
        // Drain pending candidates
        while (pendingCandidates.length > 0) {
          const cand = pendingCandidates.shift()
          try { await pc.addIceCandidate(cand) }
          catch (e) { console.warn('[webrtc] add pending candidate error', e) }
        }
      } else {
        console.warn('[webrtc] Ignoring answer in wrong signaling state:', pc.signalingState)
      }
    })

    const unsubCandidate = on('candidate', async (msg) => {
      if (msg.from !== targetID) return
      const cand = new RTCIceCandidate(msg.payload)
      if (!pc.remoteDescription) {
        pendingCandidates.push(cand)
        console.log('[webrtc] queued early remote candidate')
        return
      }
      try { await pc.addIceCandidate(cand) }
      catch (e) { console.warn('[webrtc] addIceCandidate error', e) }
    })

    // Helper to read data as text safely (handles string, ArrayBuffer, and Blob)
    const readAsText = async (data) => {
      if (typeof data === 'string') {
        return data
      }
      if (data instanceof ArrayBuffer) {
        return new TextDecoder().decode(data)
      }
      if (data instanceof Blob) {
        return await data.text()
      }
      return String(data)
    }

    console.log('[webrtc] start: created pc and sent offer to', targetID)
    logViewer('info', 'Sending SDP offer to agent')

    // Create control DataChannel (controller → agent for input events).
    const dc = pc.createDataChannel('control')
    dcRef.current = dc
    dc.onopen  = () => {
      console.log('[webrtc] DataChannel control open')
      logViewer('info', 'Control data channel opened')
    }
    dc.onerror = (e) => {
      console.error('[webrtc] DataChannel control error', e)
      logViewer('error', 'Control data channel error')
    }
    dc.onmessage = async (e) => {
      try {
        const text = await readAsText(e.data)
        const msg = JSON.parse(text)
        if (msg.type === 'displays_info') {
          console.log('[webrtc] received displays_info', msg.displays)
          setDisplays(msg.displays || [])
        } else if (msg.type === 'agent_log') {
          window.dispatchEvent(new CustomEvent('webrtc:log_received', {
            detail: {
              time: new Date().toLocaleTimeString(),
              source: 'Neev Remote Agent',
              level: msg.level || 'info',
              message: msg.message
            }
          }))
        } else if (msg.type === 'cursor_info') {
          const ci = { x: msg.x, y: msg.y, visible: msg.visible, cursorType: msg.cursorType ?? 0 }
          setCursorInfo(ci)
          window.dispatchEvent(new CustomEvent('remote:cursor_info', { detail: ci }))
        } else if (msg.type === 'wol_mac') {
          setWolEnabled(!!msg.mac)
        } else if (msg.type === 'quality') {
        setQualityInfo({ fps: msg.fps || 0, rtt: Math.round(msg.rtt || 0), loss: Math.round((msg.loss || 0) * 10) / 10, bw: msg.bw || 0 })
      } else if (msg.type === 'connection_mode') {
          const mode = msg.mode // "direct" | "stun" | "relay"
          setConnectionMode(mode)
          window.dispatchEvent(new CustomEvent('remote:connection_mode', { detail: mode }))
        } else if (msg.type === 'agent_error') {
          console.error('[webrtc] Agent error:', msg.message)
          window.dispatchEvent(new CustomEvent('webrtc:log_received', {
            detail: {
              time: new Date().toLocaleTimeString(),
              source: 'Neev Remote Agent',
              level: 'error',
              message: 'Agent Error: ' + msg.message
            }
          }))
          // Optionally notify user via alert or toast
          window.dispatchEvent(new CustomEvent('remote:agent_error', { detail: msg.message }))
        }
      } catch (err) {}
    }

    // Create clipboard DataChannel
    const clipDC = pc.createDataChannel('clipboard')
    clipRef.current = clipDC
    clipDC.onopen = () => {
      console.log('[webrtc] DataChannel clipboard open')
      logViewer('info', 'Clipboard data channel opened')
    }
    clipDC.onmessage = async (e) => {
      console.log('[webrtc] clipboard received:', e.data)
      try {
        const text = await readAsText(e.data)
        if (navigator.clipboard && navigator.clipboard.writeText) {
          navigator.clipboard.writeText(text).catch(err => console.warn('Clipboard write failed', err))
        }
      } catch (err) {
        console.warn('Clipboard read/write failed', err)
      }
    }

    // Create chat DataChannel
    const chatDC = pc.createDataChannel('chat')
    chatRef.current = chatDC
    chatDC.onopen = () => {
      console.log('[webrtc] DataChannel chat open')
      logViewer('info', 'Chat data channel opened')
    }
    chatDC.onmessage = async (e) => {
      console.log('[webrtc] chat received:', e.data)
      try {
        const text = await readAsText(e.data)
        // Dispatch custom event for chat overlays
        window.dispatchEvent(new CustomEvent('webrtc:chat_received', { detail: text }))
      } catch (err) {
        console.error('Failed to parse chat message', err)
      }
    }

    // Create file transfer DataChannel
    const fileDC = pc.createDataChannel('file_transfer')
    fileRef.current = fileDC
    fileDC.onopen = () => {
      console.log('[webrtc] DataChannel file_transfer open')
      logViewer('info', 'File transfer data channel opened')
    }
    fileDC.onmessage = (e) => console.log('[webrtc] file metadata received:', e.data)

    // Create offer.
    const offer = await pc.createOffer()
    await pc.setLocalDescription(offer)
    console.log('[webrtc] sending offer', { targetID, sdpType: offer.type })
    send({ type: 'offer', to: targetID, payload: { sdp: offer.sdp, type: offer.type } })

    cleanups.current = [unsubAnswer, unsubCandidate]
  }, [send, on])

  const stop = useCallback((targetID) => {
    if (targetID) send({ type: 'bye', to: targetID })
    cleanup()
  }, [send, cleanup])

  // sendInput serializes an event and sends it over the DataChannel.
  const sendInput = useCallback((event) => {
    const dc = dcRef.current
    if (!dc || dc.readyState !== 'open') return
    try { dc.send(JSON.stringify(event)) }
    catch (e) { console.warn('[webrtc] sendInput error', e) }
  }, [])

  const sendClipboard = useCallback((text) => {
    const dc = clipRef.current
    if (!dc || dc.readyState !== 'open') return
    try { dc.send(text) }
    catch (e) { console.warn('[webrtc] sendClipboard error', e) }
  }, [])

  const sendChat = useCallback((text) => {
    const dc = chatRef.current
    if (!dc || dc.readyState !== 'open') return
    try { dc.send(text) }
    catch (e) { console.warn('[webrtc] sendChat error', e) }
  }, [])

  const sendFile = useCallback(async (file, onProgress) => {
    const dc = fileRef.current
    if (!dc || dc.readyState !== 'open') throw new Error('File transfer channel not open')

    // Send metadata
    const meta = { action: 'start', filename: file.name, filesize: file.size }
    dc.send(JSON.stringify(meta))

    // Chunk size: 16KB (safe for WebRTC)
    const chunkSize = 16384 
    let offset = 0

    return new Promise((resolve, reject) => {
      const reader = new FileReader()
      reader.onerror = reject

      reader.onload = (e) => {
        // Wait if buffer is full (max 1MB)
        if (dc.bufferedAmount > 1024 * 1024) {
          setTimeout(() => reader.readAsArrayBuffer(file.slice(offset, offset + chunkSize)), 50)
          return
        }

        dc.send(e.target.result)
        offset += chunkSize

        if (onProgress) onProgress(Math.min(offset / file.size, 1))

        if (offset < file.size) {
          reader.readAsArrayBuffer(file.slice(offset, offset + chunkSize))
        } else {
          resolve()
        }
      }

      reader.readAsArrayBuffer(file.slice(offset, offset + chunkSize))
    })
  }, [])

  const sendWol = () => { send('wol', new ArrayBuffer(0)) }
  return { start, stop, stream, connectionState, displays, cursorInfo, connectionMode, qualityInfo, wolEnabled, sendWol, sendInput, sendClipboard, sendChat, sendFile }
}
