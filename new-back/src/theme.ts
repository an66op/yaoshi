import { createTheme } from '@mui/material/styles'
import type { PaletteMode } from '@mui/material'

const inputTypography = { fontSize: '.82rem', lineHeight: '1.4375em' }
const lightDivider = '#dce9ed'
const darkDivider = '#1b3d4d'

// Responsive border shorthands are emitted inside breakpoint rules. Keep the
// directional colours responsive too, otherwise the shorthand resets them to
// currentColor (the near-white primary text colour in dark mode).
export const responsiveSplitPanelBorderSx = {
  borderRight: { md: 1 },
  borderBottom: { xs: 1, md: 0 },
  borderRightColor: { md: 'divider' },
  borderBottomColor: { xs: 'divider', md: 'divider' },
} as const

export function createAdminTheme(mode: PaletteMode) {
  const divider = mode === 'light' ? lightDivider : darkDivider
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
      divider,
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
      MuiCard: { styleOverrides: { root: { border: `1px solid ${divider}`, boxShadow: mode === 'light' ? '0 10px 28px rgba(31,75,98,.08)' : '0 12px 30px rgba(0,0,0,.25)' } } },
      MuiTableCell: { styleOverrides: { root: { fontSize: '.78rem' }, head: { fontWeight: 700, color: mode === 'light' ? '#506b7b' : '#b8d7de', background: mode === 'light' ? '#eaf6f8' : '#123b53' } } },
      MuiDialog: { styleOverrides: { paper: { backgroundImage: 'none' } } },
      MuiInputBase: { styleOverrides: { root: inputTypography } },
      MuiInputLabel: {
        styleOverrides: {
          root: inputTypography,
          outlined: ({ ownerState }) => ({
            // Match the input's top padding, not the entire FormControl height:
            // helper text and multiline fields must not move the empty label.
            transform: ownerState.shrink
              ? 'translate(14px, -0.5625em) scale(0.75)'
              : `translate(14px, ${ownerState.size === 'small' ? '8.5' : '16.5'}px) scale(1)`,
          }),
        },
      },
      MuiTextField: { defaultProps: { size: 'small' } },
      MuiSelect: { defaultProps: { size: 'small' } },
    },
  })
}
