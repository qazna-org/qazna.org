const map = L.map('map', {
  worldCopyJump: true,
  maxZoom: 7,
  minZoom: 1.5,
  zoomSnap: 0.25
}).setView([20, 20], 2);

L.tileLayer('https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png', {
  attribution: '&copy; <a href="https://www.openstreetmap.org/">OpenStreetMap</a> contributors',
  opacity: 0.75
}).addTo(map);

const flowsLayer = L.layerGroup().addTo(map);

function addFlow(event) {
  const from = [event.from.lat, event.from.lon];
  const to = [event.to.lat, event.to.lon];
  const amount = event.amount || 0;
  const weight = Math.max(1.5, Math.log10(Math.max(amount, 10)));
  const color = '#1dd3b0';

  const polyline = L.polyline([from, to], {
    color,
    weight,
    opacity: 0.7,
    smoothFactor: 1.5
  }).addTo(flowsLayer);

  const labelFrom = event.from.name || 'Origin';
  const labelTo = event.to.name || 'Destination';
  const amountMajor = (amount / 100).toLocaleString(undefined, { maximumFractionDigits: 2 });

  polyline.bindPopup(
    `<strong>${labelFrom}</strong> → <strong>${labelTo}</strong><br>` +
      `${amountMajor} ${event.currency}`
  );

  setTimeout(() => flowsLayer.removeLayer(polyline), 60000);
}

const source = new EventSource('/v1/stream');
source.onmessage = (event) => {
  try {
    const data = JSON.parse(event.data);
    addFlow(data);
  } catch (err) {
    console.warn('Failed to parse stream payload', err);
  }
};

source.onerror = () => {
  console.warn('Stream connection lost, retrying...');
};
