import { Button } from 'antd'
import { useLingui } from '@lingui/react/macro'
import { TEMPLATE_STARTERS, type TemplateStarter } from './starters'

interface StartFromPickerProps {
  /** Which starter is currently applied; '' means the blank default tree. */
  selectedId: string
  onPick: (starter: TemplateStarter | null) => void
}

/**
 * The starting-point gallery for a NEW template.
 *
 * Without it the visual editor opens on the same generic "Welcome to Notifuse" tree for everyone,
 * which is fine for a demo and useless for a shop that sends four recognisable kinds of campaign.
 * The picker is deliberately a one-shot choice made before editing starts: it replaces the whole
 * tree, so offering it next to a template already being edited would invite losing work.
 */
export function StartFromPicker({ selectedId, onPick }: StartFromPickerProps) {
  const { t } = useLingui()

  return (
    <div className="mb-6">
      <div className="flex items-baseline gap-2 mb-2">
        <span className="font-medium">{t`Start from`}</span>
        <span className="text-xs text-gray-500">
          {t`Choosing a design replaces the current canvas. You can then edit every block normally.`}
        </span>
      </div>
      <div className="grid grid-cols-2 lg:grid-cols-5 gap-2">
        <button
          type="button"
          onClick={() => onPick(null)}
          className={`text-left border rounded-md p-3 transition-colors ${
            selectedId === ''
              ? 'border-black ring-1 ring-black'
              : 'border-gray-200 hover:border-gray-400'
          }`}
        >
          <div className="font-medium text-sm">{t`Blank`}</div>
          <div className="text-xs text-gray-500 mt-1">
            {t`An empty canvas with the default styling`}
          </div>
        </button>
        {TEMPLATE_STARTERS.map((starter) => (
          <button
            key={starter.id}
            type="button"
            onClick={() => onPick(starter)}
            className={`text-left border rounded-md p-3 transition-colors ${
              selectedId === starter.id
                ? 'border-black ring-1 ring-black'
                : 'border-gray-200 hover:border-gray-400'
            }`}
          >
            <div className="font-medium text-sm">{starter.name}</div>
            <div className="text-xs text-gray-500 mt-1 leading-snug">{starter.description}</div>
          </button>
        ))}
      </div>
      {selectedId !== '' && (
        <Button type="link" size="small" className="!px-0 mt-1" onClick={() => onPick(null)}>
          {t`Remove the design and start blank`}
        </Button>
      )}
    </div>
  )
}
