import { cookies } from 'next/headers';

export async function getAdminKey() {
  const cookieStore = await cookies();
  return cookieStore.get('qwenpi_key')?.value || null;
}
