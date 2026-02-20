import { useTranslation } from 'react-i18next'
import { Shield, ShieldCheck, ShieldAlert, Lock } from 'lucide-react'
import type { SetupData } from './SetupWizard'
import {
  StepContainer, StepHeader, FieldGroup, Field,
  PasswordInput, Toggle, SelectableCard,
} from './SetupComponents'

interface Props {
  data: SetupData
  updateData: (partial: Partial<SetupData>) => void
}

export function StepSecurity({ data, updateData }: Props) {
  const { t } = useTranslation()

  // Derive toggle state from data: different passwords = custom vault enabled
  const hasCustomVault = data.vaultPassword !== data.webuiPassword

  const MODES = [
    {
      value: 'relaxed' as const,
      label: t('setupPage.modeRelaxed'),
      description: t('setupPage.modeRelaxedDesc'),
      icon: Shield,
    },
    {
      value: 'strict' as const,
      label: t('setupPage.modeStrict'),
      description: t('setupPage.modeStrictDesc'),
      icon: ShieldCheck,
    },
    {
      value: 'paranoid' as const,
      label: t('setupPage.modeParanoid'),
      description: t('setupPage.modeParanoidDesc'),
      icon: ShieldAlert,
    },
  ]

  const handlePasswordChange = (val: string) => {
    if (hasCustomVault) {
      // Custom vault enabled - only update webui password
      updateData({ webuiPassword: val })
    } else {
      // Sync both passwords
      updateData({ webuiPassword: val, vaultPassword: val })
    }
  }

  const handleToggleVault = (enabled: boolean) => {
    if (enabled) {
      // Clear vault password so it becomes different from webui password
      updateData({ vaultPassword: '' })
    } else {
      // Sync vault password to webui password
      updateData({ vaultPassword: data.webuiPassword })
    }
  }

  return (
    <StepContainer>
      <StepHeader
        title={t('setupPage.securityTitle')}
        description={t('setupPage.securityDesc')}
      />

      <FieldGroup>
        <Field label={t('setupPage.password')} icon={Lock} hint={t('setupPage.passwordHint')}>
          <PasswordInput
            value={data.webuiPassword}
            onChange={handlePasswordChange}
            placeholder={t('setupPage.password')}
          />
        </Field>

        <Toggle
          enabled={hasCustomVault}
          onChange={handleToggleVault}
          label={t('setupPage.vaultDifferent')}
        />

        {hasCustomVault && (
          <Field label={t('setupPage.vaultPassword')} icon={Lock}>
            <PasswordInput
              value={data.vaultPassword}
              onChange={(val) => updateData({ vaultPassword: val })}
              placeholder={t('setupPage.vaultPassword')}
            />
          </Field>
        )}

        <Field label={t('setupPage.accessMode')} icon={Shield}>
          <div className="space-y-2">
            {MODES.map((mode) => (
              <SelectableCard
                key={mode.value}
                selected={data.accessMode === mode.value}
                onClick={() => updateData({ accessMode: mode.value })}
                icon={mode.icon}
                iconColor="text-[#3b82f6]"
                title={mode.label}
                description={mode.description}
              />
            ))}
          </div>
        </Field>
      </FieldGroup>
    </StepContainer>
  )
}
