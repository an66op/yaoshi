import { createTheme } from '@mui/material/styles'
import type { PaletteMode } from '@mui/material'

export function createAdminTheme(mode: PaletteMode) {
  return createTheme({
    palette: {
      mode,
      primary: { main: '#168aaa', light: '#55c7c7', dark: '#0c658b' },
      secondary: { main: '#27b8aa' },
      success: { main: '#25a978' },
      warning: { main: '#e6a23c' },
      error: { main: '#df6965' },
      background: mode === 'light' ? { default: '#edf4f7', paper: '#ffffff' } : { default: '#071a2e', paper: '#0d2942' },
      text: mode === 'light' ? { primary: '#18384d', secondary: '#718898' } : { primary: '#e4f4f7', secondary: '#93b2bc' },
      divider: mode === 'light' ? '#dce9ed' : '#244a5c',
    },
    shape: { borderRadius: 12 },
    typography: {
      fontFamily: 'Inter, "Microsoft YaHei", system-ui, sans-serif',
      fontSize: 14,
      h4: { fontWeight: 750, fontSize: '1.45rem' },
      h5: { fontWeight: 700 },
      h6: { fontWeight: 700 },
      button: { textTransform: 'none', fontWeight: 650 },
    },
    components: {
      MuiCssBaseline: { styleOverrides: { body: { margin: 0, WebkitFontSmoothing: 'antialiased' }, '*': { boxSizing: 'border-box' }, '::selection': { background: mode === 'light' ? '#bce9eb' : '#17617a' } } },
      MuiPaper: { styleOverrides: { root: { backgroundImage: 'none' } } },
      MuiButton: { defaultProps: { disableElevation: true }, styleOverrides: { root: { borderRadius: 9, minHeight: 36 } } },
      MuiCard: { styleOverrides: { root: { border: `1px solid ${mode === 'light' ? '#dce9ed' : '#244a5c'}`, boxShadow: mode === 'light' ? '0 10px 28px rgba(31,75,98,.08)' : '0 12px 30px rgba(0,0,0,.25)' } } },
      MuiTableCell: { styleOverrides: { root: { fontSize: '.78rem' }, head: { fontWeight: 700, color: mode === 'light' ? '#506b7b' : '#b8d7de', background: mode === 'light' ? '#eaf6f8' : '#123b53' } } },
      MuiDialog: { styleOverrides: { paper: { backgroundImage: 'none' } } },
      MuiInputBase: { styleOverrides: { root: { fontSize: '.82rem' } } },
      MuiTextField: { defaultProps: { size: 'small' } },
      MuiSelect: { defaultProps: { size: 'small' } },
    },
  })
}
