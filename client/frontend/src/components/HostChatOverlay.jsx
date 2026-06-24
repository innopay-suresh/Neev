import React, { useState, useEffect, useRef } from 'react'
import { motion, AnimatePresence } from 'framer-motion'
import { X, Send } from 'lucide-react'
import { SendHostChat } from '../../wailsjs/go/backend/App.js'
import styles from './HostChatOverlay.module.css'

export function HostChatOverlay() {
  const [messages, setMessages] = useState([])
  const [isOpen, setIsOpen] = useState(false)
  const [input, setInput] = useState('')
  const msgsEndRef = useRef(null)

  useEffect(() => {
    if (typeof window !== 'undefined' && window.runtime) {
      window.runtime.EventsOn('host:chat_received', (msg) => {
        setMessages(prev => [...prev, { from: 'Admin', text: msg }])
        setIsOpen(true)
      })
    }
  }, [])

  useEffect(() => {
    msgsEndRef.current?.scrollIntoView({ behavior: 'smooth' })
  }, [messages, isOpen])

  const handleSend = async (e) => {
    e.preventDefault()
    if (!input.trim()) return
    const msg = input.trim()
    setMessages(prev => [...prev, { from: 'You', text: msg }])
    setInput('')
    try {
      await SendHostChat(msg)
    } catch (err) {
      console.error(err)
    }
  }

  return (
    <>
      <AnimatePresence>
        {!isOpen && (
          <motion.button
            className={styles.fab}
            onClick={() => setIsOpen(true)}
            initial={{ scale: 0 }}
            animate={{ scale: 1 }}
            exit={{ scale: 0 }}
            title="Open Chat"
          >
            <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
              <path d="M21 11.5a8.38 8.38 0 0 1-.9 3.8 8.5 8.5 0 0 1-7.6 4.7 8.38 8.38 0 0 1-3.8-.9L3 21l1.9-5.7a8.38 8.38 0 0 1-.9-3.8 8.5 8.5 0 0 1 4.7-7.6 8.38 8.38 0 0 1 3.8-.9h.5a8.48 8.48 0 0 1 8 8v.5z" />
            </svg>
            {messages.length > 0 && <span className={styles.badge}>{messages.length}</span>}
          </motion.button>
        )}
      </AnimatePresence>

      <AnimatePresence>
        {isOpen && (
          <motion.div 
            className={styles.overlay}
            initial={{ opacity: 0, y: 20 }}
            animate={{ opacity: 1, y: 0 }}
            exit={{ opacity: 0, y: 20 }}
          >
            <div className={styles.header}>
              <h4>IT Support Chat</h4>
              <button onClick={() => setIsOpen(false)}><X size={14} /></button>
            </div>
            <div className={styles.body}>
              {messages.map((m, i) => (
                <div key={i} className={`${styles.bubble} ${m.from === 'You' ? styles.bubbleSelf : styles.bubbleAdmin}`}>
                  <strong>{m.from}:</strong> {m.text}
                </div>
              ))}
              <div ref={msgsEndRef} />
            </div>
            <form className={styles.footer} onSubmit={handleSend}>
              <input 
                value={input} 
                onChange={e => setInput(e.target.value)} 
                placeholder="Reply to admin..." 
              />
              <button type="submit"><Send size={14} /></button>
            </form>
          </motion.div>
        )}
      </AnimatePresence>
    </>
  )
}
