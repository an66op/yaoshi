import { Box, FormLabel, MenuItem, TextField, type PaletteMode } from '@mui/material'
import { ThemeProvider } from '@mui/material/styles'
import { renderToStaticMarkup } from 'react-dom/server'
import { describe, expect, it } from 'vitest'
import { createAdminTheme, responsiveSplitPanelBorderSx } from './theme'

type FieldKind = 'input' | 'select' | 'multiline'
type FieldSize = 'small' | 'medium'
type FieldState = 'empty' | 'focused' | 'filled'

function elementWithClass(html: string, className: string) {
  const element = [...html.matchAll(/<[a-z][\w-]*\b[^>]*>/g)].find(([tag]) => {
    const classes = tag.match(/\bclass="([^"]*)"/)?.[1].split(/\s+/)
    return classes?.includes(className)
  })?.[0]
  expect(element, `rendered element with ${className}`).toBeDefined()
  return element!
}

// Read the declarations attached to the actual SSR element, not unused Emotion
// style blocks or the theme configuration. Include MUI's compound class rules
// (e.g. Select's height reset), ordered by specificity and then source order.
function stylesFor(html: string, className: string) {
  const element = elementWithClass(html, className)
  const classes = element.match(/\bclass="([^"]*)"/)?.[1].split(/\s+/) ?? []
  const emotionClass = classes.find(name => name.startsWith('css-'))
  expect(emotionClass, `Emotion class on ${className}`).toBeDefined()
  const css = [...html.matchAll(/<style\b[^>]*>([\s\S]*?)<\/style>/g)].map(match => match[1]).join('')
  const declarations = [...css.matchAll(/([^{}]+)\{([^{}]*)\}/g)]
    .map(match => ({ selector: match[1].trim(), body: match[2] }))
    .filter(({ selector }) => /^(?:\.[\w-]+)+$/.test(selector))
    .map(rule => ({ ...rule, classes: rule.selector.slice(1).split('.') }))
    .filter(rule => rule.classes.includes(emotionClass!) && rule.classes.every(name => classes.includes(name)))
    .sort((left, right) => left.classes.length - right.classes.length)
    .flatMap(rule => rule.body.split(';'))
    .filter(Boolean)
    .map(declaration => {
      const colon = declaration.indexOf(':')
      return [declaration.slice(0, colon), declaration.slice(colon + 1)]
    })
  expect(declarations.length, `SSR declarations for ${className}`).toBeGreaterThan(0)
  return Object.fromEntries(declarations) as Record<string, string | undefined>
}

function renderField(mode: PaletteMode, size: FieldSize, kind: FieldKind, state: FieldState) {
  return renderToStaticMarkup(
    <ThemeProvider theme={createAdminTheme(mode)}>
      <TextField
        id="alignment-field"
        label="输入标签"
        helperText="辅助说明不应影响标签位置"
        variant="outlined"
        size={size}
        value={state === 'filled' ? 'value' : ''}
        onChange={() => undefined}
        focused={state === 'focused'}
        select={kind === 'select'}
        slotProps={kind === 'select' ? { select: { size } } : undefined}
        multiline={kind === 'multiline'}
        minRows={kind === 'multiline' ? 3 : undefined}
      >
        {kind === 'select' ? <MenuItem value="value">选项</MenuItem> : undefined}
      </TextField>
    </ThemeProvider>,
  )
}

const cases = (['small', 'medium'] as const).flatMap(size =>
  (['input', 'select', 'multiline'] as const).flatMap(kind =>
    (['empty', 'focused', 'filled'] as const).map(state => ({ size, kind, state })),
  ),
)

describe.each(['light', 'dark'] as const)('admin input label alignment in %s mode', mode => {
  it.each(cases)('aligns the $size $kind label when $state', ({ size, kind, state }) => {
    const html = renderField(mode, size, kind, state)
    const label = elementWithClass(html, 'MuiInputLabel-root')
    const labelStyles = stylesFor(html, 'MuiInputLabel-root')
    const inputStyles = stylesFor(html, 'MuiInputBase-root')
    const floating = state !== 'empty'

    expect(label).toContain('for="alignment-field"')
    expect(label).toContain(`data-shrink="${floating}"`)
    expect(label.includes('MuiInputLabel-shrink')).toBe(floating)
    expect(label.includes('Mui-focused')).toBe(state === 'focused')
    expect(labelStyles['font-size']).toBe('.82rem')
    expect(labelStyles['font-size']).toBe(inputStyles['font-size'])
    expect(labelStyles['line-height']).toBe('1.4375em')
    expect(labelStyles['line-height']).toBe(inputStyles['line-height'])
    expect(labelStyles.transform).toBe(floating
      ? 'translate(14px, -0.5625em) scale(0.75)'
      : `translate(14px, ${size === 'small' ? '8.5' : '16.5'}px) scale(1)`)
    expect(labelStyles.top).toBe('0')
    expect(labelStyles.left).toBe('0')

    // The notch inherits the same base font as the input. Its 0.75em text must
    // continue to match the floating label's 0.75 scale, including selects.
    const legendClass = html.match(/<legend\b[^>]*class="([^"]+)"/)?.[1]
    expect(legendClass).toBeDefined()
    const legendStyles = stylesFor(html, legendClass!)
    expect(legendStyles['font-size']).toBe('0.75em')
    expect(legendStyles['max-width']).toBe(floating ? '100%' : '0.01px')

    const controlStyles = stylesFor(html, 'MuiOutlinedInput-input')
    const padding = `${size === 'small' ? '8.5' : '16.5'}px 14px`
    if (kind === 'multiline') {
      expect(elementWithClass(html, 'MuiOutlinedInput-input')).toMatch(/^<textarea\b/)
      expect(inputStyles.padding).toBe(padding)
      expect(inputStyles.height).toBeUndefined()
      expect(controlStyles.padding).toBe('0')
      expect(controlStyles.height).toBe('auto')
      expect(controlStyles.resize).toBe('none')
    } else {
      expect(controlStyles.padding).toBe(padding)
      expect(controlStyles.height).toBe(kind === 'select' ? 'auto' : '1.4375em')
      if (kind === 'select') {
        expect(elementWithClass(html, 'MuiSelect-select')).toContain('role="combobox"')
        expect(controlStyles['min-height']).toBe('1.4375em')
      }
    }
    expect(html).toContain('辅助说明不应影响标签位置')
  })

  it('does not turn group labels into positioned input labels', () => {
    const html = renderToStaticMarkup(
      <ThemeProvider theme={createAdminTheme(mode)}>
        <FormLabel component="legend">复选框分组</FormLabel>
      </ThemeProvider>,
    )
    const groupStyles = stylesFor(html, 'MuiFormLabel-root')
    expect(groupStyles['font-size']).toBe('1rem')
    expect(groupStyles.position).toBe('relative')
    expect(groupStyles.transform).toBeUndefined()
    expect(groupStyles.top).toBeUndefined()
  })

  it.each(['standard', 'filled'] as const)('limits the outlined transform adjustment to outlined fields, not %s', variant => {
    const html = renderToStaticMarkup(
      <ThemeProvider theme={createAdminTheme(mode)}>
        <TextField label="其他样式" variant={variant} size="small" />
      </ThemeProvider>,
    )
    expect(stylesFor(html, 'MuiInputLabel-root').transform).toBe(variant === 'standard'
      ? 'translate(0, 17px) scale(1)'
      : 'translate(12px, 13px) scale(1)')
  })
})

describe('admin surface borders', () => {
  it('keeps light borders unchanged and uses a soft blue-grey in dark mode', () => {
    const lightTheme = createAdminTheme('light')
    const darkTheme = createAdminTheme('dark')

    expect(lightTheme.palette.divider).toBe('#dce9ed')
    expect(darkTheme.palette.divider).toBe('#1b3d4d')
    expect(lightTheme.components?.MuiCard?.styleOverrides?.root).toMatchObject({ border: '1px solid #dce9ed' })
    expect(darkTheme.components?.MuiCard?.styleOverrides?.root).toMatchObject({ border: '1px solid #1b3d4d' })
  })

  it('reapplies the divider colour inside responsive split-panel rules', () => {
    const html = renderToStaticMarkup(
      <ThemeProvider theme={createAdminTheme('dark')}>
        <Box sx={responsiveSplitPanelBorderSx} />
      </ThemeProvider>,
    )

    expect(html).toContain('border-right-color:#1b3d4d')
    expect(html).toContain('border-bottom-color:#1b3d4d')
    expect(html).not.toContain('border-right-color:#e4f4f7')
  })
})
