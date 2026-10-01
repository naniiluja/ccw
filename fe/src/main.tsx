import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { createApp } from '@/app/create-app'
import './index.css'

const { Root } = createApp()

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <Root />
  </StrictMode>,
)
