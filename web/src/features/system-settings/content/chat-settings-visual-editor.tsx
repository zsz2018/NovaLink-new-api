/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { Plus, Search } from 'lucide-react'
import { useState, useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import { StaticDataTable } from '@/components/data-table/static/static-data-table'
import { StaticRowActions } from '@/components/data-table/static/static-row-actions'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import { ChatPresetIcon } from '@/features/chat/components/chat-preset-icon'
import {
  MAX_CHAT_SHORTCUTS,
  parseChatConfig,
  type ChatPreset,
} from '@/features/chat/lib/chat-links'

import { safeJsonParse } from '../utils/json-parser'
import { ChatDialog, type ChatEntryData } from './chat-dialog'

type ChatSettingsVisualEditorProps = {
  value: string
  onChange: (value: string) => void
}

export function ChatSettingsVisualEditor({
  value,
  onChange,
}: ChatSettingsVisualEditorProps) {
  const { t } = useTranslation()
  const [searchText, setSearchText] = useState('')
  const [dialogOpen, setDialogOpen] = useState(false)
  const [editData, setEditData] = useState<ChatPreset | null>(null)

  const entries = useMemo(
    () => safeJsonParse<unknown>(value || '[]', { silent: true }),
    [value]
  )
  const chats = useMemo(
    () => (Array.isArray(entries) ? parseChatConfig(entries) : []),
    [entries]
  )
  const atLimit =
    chats.filter((chat) => chat.shortcut).length >= MAX_CHAT_SHORTCUTS

  const filteredChats = useMemo(() => {
    if (!searchText) return chats
    const lowerSearch = searchText.toLowerCase()
    return chats.filter(
      (chat) =>
        chat.name.toLowerCase().includes(lowerSearch) ||
        chat.url.toLowerCase().includes(lowerSearch)
    )
  }, [chats, searchText])

  function updateEntry(id: string | undefined, data?: ChatEntryData) {
    if (!Array.isArray(entries)) return
    const updated = [...entries]
    if (id === undefined) updated.push(data)
    else if (!data) updated.splice(Number(id), 1)
    else updated[Number(id)] = data
    onChange(JSON.stringify(updated, null, 2))
  }

  const handleEdit = (chat: ChatPreset) => {
    setEditData(chat)
    setDialogOpen(true)
  }

  const handleAdd = () => {
    setEditData(null)
    setDialogOpen(true)
  }

  return (
    <div className='space-y-4'>
      <div className='flex items-center gap-4'>
        <div className='relative flex-1'>
          <Search className='text-muted-foreground absolute top-2.5 left-2.5 h-4 w-4' />
          <Input
            placeholder={t('Search chat presets...')}
            value={searchText}
            onChange={(e) => setSearchText(e.target.value)}
            className='pl-9'
          />
        </div>
        <Button
          type='button'
          onClick={handleAdd}
          disabled={!Array.isArray(entries)}
        >
          <Plus className='mr-2 h-4 w-4' />
          {t('Add chat preset')}
        </Button>
      </div>

      <StaticDataTable
        data={filteredChats}
        getRowKey={(chat) => chat.id}
        emptyContent={
          searchText
            ? t('No chat presets match your search')
            : t(
                'No chat presets configured. Click "Add chat preset" to get started.'
              )
        }
        columns={[
          {
            id: 'name',
            header: t('Chat Client Name'),
            cellClassName: 'font-medium',
            cell: (chat) => (
              <span className='inline-flex items-center gap-2'>
                <ChatPresetIcon name={chat.name} icon={chat.icon} />
                {chat.name}
              </span>
            ),
          },
          {
            id: 'url',
            header: t('URL'),
            cellClassName: 'max-w-md truncate font-mono text-sm',
            cell: (chat) => chat.url,
          },
          {
            id: 'shortcut',
            header: t('API key shortcut'),
            cell: (chat) => (
              <Checkbox
                aria-label={t('Import to {{name}}', { name: chat.name })}
                checked={chat.shortcut}
                disabled={!chat.shortcut && atLimit}
                onCheckedChange={(shortcut) =>
                  updateEntry(chat.id, {
                    name: chat.name,
                    url: chat.url,
                    icon: chat.icon,
                    shortcut,
                  })
                }
              />
            ),
          },
          {
            id: 'actions',
            header: t('Actions'),
            className: 'text-right',
            cellClassName: 'text-right',
            cell: (chat) => (
              <StaticRowActions
                editLabel={t('Edit')}
                deleteLabel={t('Delete')}
                menuLabel={t('Open menu')}
                onEdit={() => handleEdit(chat)}
                onDelete={() => updateEntry(chat.id)}
              />
            ),
          },
        ]}
      />

      <ChatDialog
        open={dialogOpen}
        onOpenChange={setDialogOpen}
        onSave={(data) => updateEntry(editData?.id, data)}
        editData={editData}
      />
    </div>
  )
}
