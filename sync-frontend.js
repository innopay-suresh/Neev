#!/usr/bin/env node

/**
 * sync-frontend.js
 * Automatically copies shared hooks and components from client/frontend to web
 * to prevent code drift between the Desktop Wails client and Web client.
 */

const fs = require('fs');
const path = require('path');

const ROOT_DIR = __dirname;
const SRC_DIR = path.join(ROOT_DIR, 'client', 'frontend', 'src');
const DEST_DIR = path.join(ROOT_DIR, 'web', 'src');

const SHARED_FILES = [
  { from: 'hooks/useInputCapture.js', to: 'hooks/useInputCapture.js' },
  { from: 'hooks/useWebRTC.js', to: 'hooks/useWebRTC.js' },
  { from: 'components/SessionView.jsx', to: 'components/SessionView.jsx' },
  { from: 'components/SessionView.module.css', to: 'components/SessionView.module.css' }
];

console.log('🔄 Syncing shared frontend assets and hooks...');

SHARED_FILES.forEach(({ from, to }) => {
  const sourcePath = path.join(SRC_DIR, from);
  const destPath = path.join(DEST_DIR, to);

  try {
    // Ensure destination directory exists
    const destDir = path.dirname(destPath);
    if (!fs.existsSync(destDir)) {
      fs.mkdirSync(destDir, { recursive: true });
    }

    // Copy file
    fs.copyFileSync(sourcePath, destPath);
    console.log(`✅ Synced: ${from} -> ${to}`);
  } catch (err) {
    console.error(`❌ Failed to sync ${from}:`, err.message);
  }
});

console.log('✨ Synchronization complete.');
