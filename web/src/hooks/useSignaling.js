/**
 * useSignaling — manages the WebSocket connection to the signaling server.
 *
 * Features:
 *  - Auto-reconnect with exponential backoff (up to 30s)
 *  - Message routing via on(type, handler) subscriptions
 *  - JSON serialization / deserialization
 */

import { useEffect, useRef, useCallback, useState } from 'react'

const MAX_BACKOFF_MS = 30_000

export function useSignaling(relayURL, options = {}) {
  const { log } = options
  const wsRef       = useRef(null)
  const [connected, setConnected] = useState(false)
  const handlersRef = useRef({})
  const backoffRef  = useRef(1000)
  const reconnTimer = useRef(null)

  const send = useCallback((msg) => {
    if (wsRef.current?.readyState === WebSocket.OPEN) {
      wsRef.current.send(JSON.stringify(msg))
      log?.('debug', 'signaling', `sent ${msg.type}`, msg)
    } else {
      console.warn('[signaling] send dropped — not connected', msg)
      log?.('warn', 'signaling', `dropped ${msg.type} because socket is closed`, msg)
    }
  }, [log])

  const on = useCallback((type, handler) => {
    if (!handlersRef.current[type]) handlersRef.current[type] = new Set()
    handlersRef.current[type].add(handler)
    return () => handlersRef.current[type]?.delete(handler)
  }, [])

  useEffect(() => {
    let dead = false

    const connect = () => {
      if (dead) return
      try {
        const ws = new WebSocket(relayURL)
        wsRef.current = ws

        ws.onopen = () => {
          backoffRef.current = 1000
          setConnected(true)
          log?.('info', 'signaling', 'connected to relay', { relayURL })
        }

        ws.onmessage = (evt) => {
          let msg
          try { msg = JSON.parse(evt.data) } catch { return }
          const handlers = handlersRef.current[msg.type]
          log?.(msg.type === 'error' ? 'error' : 'debug', 'signaling', `received ${msg.type}`, msg)
          if (handlers) handlers.forEach(h => h(msg))
        }

        ws.onerror = () => {
          log?.('warn', 'signaling', 'websocket error encountered')
        } // onclose fires next

        ws.onclose = () => {
          setConnected(false)
          log?.('warn', 'signaling', 'relay connection closed')
          if (dead) return
          const delay = backoffRef.current
          backoffRef.current = Math.min(backoffRef.current * 2, MAX_BACKOFF_MS)
          reconnTimer.current = setTimeout(connect, delay)
        }
      } catch (e) {
        console.error('[signaling] connect error', e)
        log?.('error', 'signaling', 'failed to connect to relay', { error: String(e) })
      }
    }

    connect()
    return () => {
      dead = true
      clearTimeout(reconnTimer.current)
      wsRef.current?.close()
    }
  }, [relayURL, log])

  return { connected, send, on }
}
