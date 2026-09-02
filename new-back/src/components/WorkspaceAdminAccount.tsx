import { Alert, Button, Dialog, DialogActions, DialogContent, DialogTitle, Link, Stack, TextField, Typography } from '@mui/material'
import { managementLoginURL, workspaceAdminRoleLabel, type CreatedWorkspaceAdmin, type WorkspaceAdminRole } from '../utils/workspaceAdminAccount'

const loginURL = () => managementLoginURL(typeof window === 'undefined' ? undefined : window.location.origin)

export function WorkspaceAdminLoginHint({ role }: { role: WorkspaceAdminRole }) {
  return <Alert severity="info">
    {workspaceAdminRoleLabel(role)}账号本身即可登录管理后台；在这里设置账号和密码，无需另外创建管理员账号。房间作为账号的附属配置一并开通。
    <Typography component="div" variant="body2" mt={.5}>后台入口：<Link href={loginURL()} target="_blank" rel="noopener noreferrer">{loginURL()}</Link></Typography>
  </Alert>
}

export function WorkspaceAdminAccountFields({ role, username, password, onUsernameChange, onPasswordChange, editing = false, disabled = false }: {
  role: WorkspaceAdminRole
  username: string
  password: string
  onUsernameChange: (value: string) => void
  onPasswordChange: (value: string) => void
  editing?: boolean
  disabled?: boolean
}) {
  return <Stack gap={1.5}>
    {!editing && <WorkspaceAdminLoginHint role={role} />}
    <TextField required fullWidth label={`${workspaceAdminRoleLabel(role)}登录账号`} value={username} disabled={editing || disabled} autoComplete="off" onChange={event => onUsernameChange(event.target.value)} helperText={editing ? '登录账号创建后不可修改；密码可在列表中重置' : '用于管理后台登录，3–50 个字符，全平台唯一；不是房间号'} />
    {!editing && <TextField required fullWidth type="password" label="初始登录密码" value={password} disabled={disabled} autoComplete="new-password" onChange={event => onPasswordChange(event.target.value)} helperText="8–72 字节，中文等字符占多个字节；请妥善保存" />}
  </Stack>
}

export function WorkspaceAdminCreatedDialog({ account, onClose }: { account: CreatedWorkspaceAdmin | null; onClose: () => void }) {
  return <Dialog open={Boolean(account)} onClose={onClose} fullWidth maxWidth="sm" aria-labelledby="workspace-admin-created-title">
    <DialogTitle id="workspace-admin-created-title">{account ? workspaceAdminRoleLabel(account.role) : ''}账号已创建</DialogTitle>
    {account && <DialogContent><Stack gap={1.5}>
      <Alert severity={account.status === 1 ? 'success' : 'warning'}>{account.status === 1 ? '账号已创建，可使用创建时设置的密码登录管理后台。' : '账号已创建，但当前已停用；启用后才能登录管理后台。'}</Alert>
      <Typography>登录账号：<strong>{account.username}</strong></Typography>
      <Typography>账号身份：{workspaceAdminRoleLabel(account.role)}</Typography>
      <Typography>房间号：{account.roomCode}</Typography>
      <Typography sx={{ overflowWrap: 'anywhere' }}>后台入口：<Link href={loginURL()} target="_blank" rel="noopener noreferrer">{loginURL()}</Link></Typography>
      <Typography variant="body2" color="text.secondary">使用账号和密码登录，身份由系统识别；账号资料在{workspaceAdminRoleLabel(account.role)}管理中维护，忘记密码时可使用“重置密码”。</Typography>
      <Typography variant="body2" color="text.secondary">测试新账号时，请先退出当前账号，或用无痕窗口打开后台。</Typography>
      <Typography variant="body2" color="text.secondary">此处不回显初始密码，也不会把密码存入浏览器。</Typography>
    </Stack></DialogContent>}
    <DialogActions><Button variant="contained" onClick={onClose}>我知道了</Button></DialogActions>
  </Dialog>
}
