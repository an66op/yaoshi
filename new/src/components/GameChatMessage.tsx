import type { ChatMessage } from '../api/chat'
import { compactAcceptedReceiptContent, formatGameMessageTime } from '../utils/gameRoomMessages'
import { Avatar } from './Avatar'

export function GameChatMessage({ message, nickname }: { message: ChatMessage; nickname: string }) {
  const time = <time className={`game-message-time${message.mine ? ' mine' : ''}`} dateTime={message.created_at}>{formatGameMessageTime(message.created_at)}</time>
  if (message.mine) return <div className="player-bet game-chat-message mine"><div><small>{nickname}</small><article><span className="game-chat-content">{message.content}</span>{time}</article></div><Avatar className="player-avatar" index={-1} label="我的头像" /></div>

  if (['application', 'settlement', 'scoreboard'].includes(message.message_type)) {
    const displayContent = message.user_id === 0 && message.message_type === 'application'
      ? compactAcceptedReceiptContent(message.content)
      : message.content
    const [mention, ...content] = displayContent.split('\n')
    const lines = mention.startsWith('@') ? content : [mention, ...content]
    const tone = message.message_type === 'settlement' ? ' room-settlement-message' : message.message_type === 'scoreboard' ? ' room-scoreboard-message' : ''
    return <div className={`admin-message application-assistant-message${tone}`}>
      <span className="service-logo draw-assistant-logo"><img alt="开奖助手头像" src="/images/draw-assistant-avatar-v1.jpg" /></span>
      <div><small>开奖助手 · 24小时在线</small><article>
        {mention.startsWith('@') && <span className="assistant-mention">{mention}</span>}
        {lines.map((line, index) => index === 0
          ? <strong key={`${message.id}-${index}`}>{line}</strong>
          : line ? <span className={`assistant-response-line${line.startsWith('得分：+') ? ' positive' : line.startsWith('得分：-') ? ' negative' : line.startsWith('[') ? ' player-row' : ''}`} key={`${message.id}-${index}`}>{line}</span>
            : <i className="assistant-response-gap" key={`${message.id}-${index}`} />)}
        {time}
      </article></div>
    </div>
  }

  return <article className="market-bet game-chat-message"><Avatar index={Number(message.public_id ?? message.user_id ?? 0)} label={`${message.nickname}的头像`} /><div><small>{message.nickname}</small><p><span className="game-chat-content">{message.content}</span>{time}</p></div></article>
}
