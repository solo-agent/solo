// Solo Service Worker — handles Web Push notifications for agent run completions.
self.addEventListener('push', (event) => {
  const data = event.data?.json() ?? { title: 'Solo', body: '' };
  event.waitUntil(
    self.registration.showNotification(data.title, {
      body: data.body,
      icon: '/favicon.svg',
      badge: '/favicon.svg',
      data: { url: data.url || '/' },
      requireInteraction: false,
      tag: 'solo-agent',
    })
  );
});

self.addEventListener('notificationclick', (event) => {
  event.notification.close();
  const url = event.notification.data?.url || '/';
  event.waitUntil(clients.openWindow(url));
});
