'use client';

import { RelationshipWorkspace } from '@/components/relationships/relationship-workspace';
import { t } from '@/lib/i18n';

export default function TeamsPage() {
  return <RelationshipWorkspace title={t('relationshipPageTitle')} />;
}
