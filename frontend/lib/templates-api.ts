import { apiClient } from '@/lib/api-client';
import { getLocale } from '@/lib/i18n';

export interface TemplateMember {
  ref: string;
  role: string;
  name: string;
  description: string;
  instructions: string;
  avatar_url: string;
}

export interface TemplateRelationship {
  from_ref: string;
  to_ref: string;
  type: 'assigns_to' | 'collaborates_with';
  weight: number;
  instruction?: string;
}

export interface Template {
  id: string;
  name: string;
  description: string;
  category: string;
  icon: string;
  member_count: number;
  roles: string[];
  avatar_urls: string[];
  members?: TemplateMember[];
  relationships?: TemplateRelationship[];
}

export async function listTemplates(): Promise<Template[]> {
  const res = await apiClient.get<Template[]>('/api/v1/templates', { lang: getLocale() });
  return Array.isArray(res) ? res : [];
}

export async function getTemplate(id: string): Promise<Template> {
  return apiClient.get<Template>(
    `/api/v1/templates/${encodeURIComponent(id)}`,
    { lang: getLocale() },
  );
}
