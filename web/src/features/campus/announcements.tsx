import { useQueryClient } from '@tanstack/react-query'
import { Bell, CheckCheck, Megaphone, Trash2 } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { Dialog } from '@/components/dialog'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'

import { useAnnouncements, type Announcement } from './announcement-data'
import { formatDate, request } from './api'
import { readPreference, writePreference } from './preferences'
import { ErrorState, Loading, PageHeading } from './shared'

export function AnnouncementBell(props: { userId?: number }) {
  const { t } = useTranslation()
  const feed = useAnnouncements()
  const [open, setOpen] = useState(false)
  const [timeline, setTimeline] = useState(false)
  const key = `platform-announcements-read:v1:${props.userId || 'public'}`
  const [seen, setSeen] = useState(() => Number(readPreference(key)) || 0)
  const items = feed.data?.items || []
  const unread = items.filter((item) => item.id > seen).length
  function markRead() {
    const latest = items[0]?.id || seen
    setSeen(latest)
    writePreference(key, String(latest))
  }
  return (
    <>
      <Button
        variant='ghost'
        size='icon'
        className='announcement-trigger'
        aria-label={t('System announcements')}
        onClick={() => setOpen(true)}
      >
        <Bell size={18} />
        {unread > 0 && (
          <span className='unread-count'>{Math.min(unread, 99)}</span>
        )}
      </Button>
      <Dialog
        open={open}
        onOpenChange={(value) => {
          setOpen(value)
          if (!value) markRead()
        }}
        title={t('System announcements')}
        description={t('Latest platform updates and notices')}
        contentClassName='announcement-dialog'
      >
        <div
          className='segmented-tabs'
          role='group'
          aria-label={t('Announcement view')}
        >
          <button
            type='button'
            aria-pressed={!timeline}
            onClick={() => setTimeline(false)}
          >
            {t('Notifications')}
          </button>
          <button
            type='button'
            aria-pressed={timeline}
            onClick={() => setTimeline(true)}
          >
            {t('Timeline')}
          </button>
        </div>
        {feed.isPending && <Loading />}
        {feed.error && (
          <ErrorState error={feed.error} retry={() => void feed.refetch()} />
        )}
        {feed.data && items.length === 0 && (
          <div className='announcement-empty'>
            <Bell size={30} />
            <p>{t('No notice')}</p>
          </div>
        )}
        {feed.data && items.length > 0 && (
          <div
            className={
              timeline ? 'announcement-list timeline' : 'announcement-list'
            }
          >
            {(timeline ? items : items.slice(0, 5)).map((item) => (
              <article key={item.id} className='announcement-item'>
                <div>
                  <span className='announcement-date'>
                    {formatDate(item.created_at)}
                  </span>
                  {item.id > seen && (
                    <span className='new-notice'>{t('New')}</span>
                  )}
                </div>
                <h3>{item.title}</h3>
                <p>{item.content}</p>
              </article>
            ))}
            {unread > 0 && (
              <Button variant='ghost' onClick={markRead}>
                <CheckCheck size={16} />
                {t('Mark all read')}
              </Button>
            )}
          </div>
        )}
      </Dialog>
    </>
  )
}
export function AdminAnnouncements() {
  const { t } = useTranslation()
  const feed = useAnnouncements()
  const cache = useQueryClient()
  const [title, setTitle] = useState('')
  const [content, setContent] = useState('')
  const [pending, setPending] = useState(false)
  const [removing, setRemoving] = useState<Announcement | null>(null)
  async function publish() {
    setPending(true)
    try {
      await request('/admin/notice', 'PUT', { title, content })
      await cache.invalidateQueries({ queryKey: ['announcements'] })
      await cache.invalidateQueries({ queryKey: ['notice'] })
      setTitle('')
      setContent('')
      toast.success(t('Published'))
    } catch (error) {
      toast.error((error as Error).message)
    } finally {
      setPending(false)
    }
  }
  async function withdraw() {
    if (!removing) return
    setPending(true)
    try {
      await request(`/admin/announcements/${removing.id}`, 'DELETE')
      await cache.invalidateQueries({ queryKey: ['announcements'] })
      await cache.invalidateQueries({ queryKey: ['notice'] })
      setRemoving(null)
      toast.success(t('Withdrawn'))
    } catch (error) {
      toast.error((error as Error).message)
    } finally {
      setPending(false)
    }
  }
  return (
    <>
      <PageHeading
        title={t('System announcements')}
        description={t(
          'Published notices appear on the homepage and in the console.'
        )}
      />
      <Card className='announcement-editor'>
        <h2>
          <Megaphone size={19} />
          {t('Publish notice')}
        </h2>
        <form
          className='editor-form'
          onSubmit={(event) => {
            event.preventDefault()
            void publish()
          }}
        >
          <label>
            {t('Title')}
            <Input
              required
              maxLength={120}
              value={title}
              onChange={(event) => setTitle(event.target.value)}
            />
          </label>
          <label>
            {t('Notice content')}
            <Textarea
              required
              rows={6}
              maxLength={8000}
              value={content}
              onChange={(event) => setContent(event.target.value)}
            />
          </label>
          <Button
            type='submit'
            disabled={pending || !title.trim() || !content.trim()}
          >
            {t(pending ? 'Publishing' : 'Publish notice')}
          </Button>
        </form>
      </Card>
      <h2 className='announcement-history-heading'>{t('Published notices')}</h2>
      {feed.isPending && <Loading />}
      {feed.error && (
        <ErrorState error={feed.error} retry={() => void feed.refetch()} />
      )}
      {feed.data && feed.data.items.length === 0 && (
        <Card className='announcement-empty'>
          <Bell size={28} />
          <p>{t('No notice')}</p>
        </Card>
      )}
      {feed.data && feed.data.items.length > 0 && (
        <div className='announcement-history'>
          {feed.data.items.map((item) => (
            <Card key={item.id} className='announcement-item'>
              <div className='announcement-history-title'>
                <h3>{item.title}</h3>
                <Button
                  variant='ghost'
                  size='icon'
                  aria-label={`${t('Withdraw notice')}: ${item.title}`}
                  onClick={() => setRemoving(item)}
                >
                  <Trash2 size={16} />
                </Button>
              </div>
              <span className='announcement-date'>
                {formatDate(item.created_at)}
              </span>
              <p>{item.content}</p>
            </Card>
          ))}
        </div>
      )}
      <ConfirmDialog
        open={!!removing}
        onOpenChange={(value) => {
          if (!value && !pending) setRemoving(null)
        }}
        title={t('Withdraw notice')}
        desc={t(
          'This notice will be removed from the homepage and notification history.'
        )}
        confirmText={t('Withdraw')}
        isLoading={pending}
        handleConfirm={() => void withdraw()}
      />
    </>
  )
}
