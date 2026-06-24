/**
 * useInputCapture — captures mouse and keyboard events from the
 * session view and forwards them via the Wails bridge.
 *
 * Mouse coordinates are normalized to 0.0–1.0 relative to the
 * video element so they are resolution-independent on the host.
 */

import { useCallback, useEffect, useRef } from 'react'

export function useInputCapture({ containerRef, videoRef, sendInput, enabled }) {
  const isEnabled = enabled && !!sendInput

  const normalize = useCallback((e) => {
    const el = containerRef.current
    const video = videoRef?.current
    if (!el || !video || !video.videoWidth) return { x: 0, y: 0 }
    
    const rect = el.getBoundingClientRect()
    
    const vW = video.videoWidth
    const vH = video.videoHeight
    
    // Calculate the scale to fit the video in the container
    const scale = Math.min(rect.width / vW, rect.height / vH)
    
    // Calculate the actual rendered dimensions of the video
    const renderedWidth = vW * scale
    const renderedHeight = vH * scale
    
    // Calculate the offsets (black bars)
    const offsetX = (rect.width - renderedWidth) / 2
    const offsetY = (rect.height - renderedHeight) / 2
    
    // Calculate the relative coordinate inside the rendered video
    const relativeX = (e.clientX - rect.left - offsetX)
    const relativeY = (e.clientY - rect.top - offsetY)
    
    // Normalize to 0-1
    return {
      x: Math.max(0, Math.min(1, relativeX / renderedWidth)),
      y: Math.max(0, Math.min(1, relativeY / renderedHeight)),
    }
  }, [containerRef, videoRef])

  const lastEventRef = useRef({ time: 0, type: '', x: 0, y: 0 })

  const checkLoop = (type, x, y) => {
    const now = Date.now()
    const last = lastEventRef.current
    if (last.type === type && last.x === x && last.y === y && (now - last.time) < 50) {
      return true
    }
    lastEventRef.current = { time: now, type, x, y }
    return false
  }

  // ── Mouse ──────────────────────────────────────────────────────────────
  const lastMoveTimeRef = useRef(0)

  const onMouseMove = useCallback((e) => {
    if (!isEnabled) return
    const now = Date.now()
    if (now - lastMoveTimeRef.current < 5) return // Throttle to ~200Hz
    
    const { x, y } = normalize(e)
    if (checkLoop('move', x, y)) return
    
    lastMoveTimeRef.current = now
    sendInput({ type: 'mouse_move', x, y })
  }, [isEnabled, normalize, sendInput])

  const onMouseDown = useCallback((e) => {
    if (!isEnabled) return
    e.preventDefault()
    const { x, y } = normalize(e)
    if (checkLoop('down', x, y)) return
    sendInput({ type: 'mouse_down', x, y, button: e.button })
  }, [isEnabled, normalize, sendInput])

  const onMouseUp = useCallback((e) => {
    if (!isEnabled) return
    const { x, y } = normalize(e)
    if (checkLoop('up', x, y)) return
    sendInput({ type: 'mouse_up', x, y, button: e.button })
  }, [isEnabled, normalize, sendInput])

  const lastWheelTimeRef = useRef(0)
  
  const onWheel = useCallback((e) => {
    if (!isEnabled) return
    e.preventDefault()
    
    const now = Date.now()
    if (now - lastWheelTimeRef.current < 16) return // Throttle scroll to ~60Hz
    lastWheelTimeRef.current = now
    
    // Smooth scrolling mice send very large deltas. 
    // Divide by 50 to get a more reasonable "lines" count for macOS.
    const dx = Math.round(e.deltaX / 50) || Math.sign(e.deltaX)
    const dy = Math.round(e.deltaY / 50) || Math.sign(e.deltaY)
    
    sendInput({ type: 'mouse_scroll', dx: dx, dy: dy })
  }, [isEnabled, sendInput])

  const onContextMenu = useCallback((e) => {
    if (isEnabled) e.preventDefault()
  }, [isEnabled])

  // ── Keyboard (global while session is active) ─────────────────────────
  useEffect(() => {
    if (!isEnabled) return

    const onKeyDown = (e) => {
      const activeTag = document.activeElement?.tagName
      if (activeTag === 'INPUT' || activeTag === 'TEXTAREA' || document.activeElement?.isContentEditable) {
        return
      }
      e.preventDefault()
      sendInput({
        type: 'key_down',
        key_code: e.keyCode,
        code: e.code,
        char: e.key.length === 1 ? e.key : '',
        modifiers:
          (e.shiftKey ? 1 : 0) |
          (e.ctrlKey  ? 2 : 0) |
          (e.altKey   ? 4 : 0) |
          (e.metaKey  ? 8 : 0),
      })
    }

    const onKeyUp = (e) => {
      const activeTag = document.activeElement?.tagName
      if (activeTag === 'INPUT' || activeTag === 'TEXTAREA' || document.activeElement?.isContentEditable) {
        return
      }
      e.preventDefault()
      sendInput({ type: 'key_up', key_code: e.keyCode, code: e.code })
    }

    window.addEventListener('keydown', onKeyDown)
    window.addEventListener('keyup',   onKeyUp)
    return () => {
      window.removeEventListener('keydown', onKeyDown)
      window.removeEventListener('keyup',   onKeyUp)
    }
  }, [isEnabled, sendInput])

  return { onMouseMove, onMouseDown, onMouseUp, onWheel, onContextMenu }
}
