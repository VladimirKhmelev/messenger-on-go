import { state } from '../state.js';
import { escapeHtml } from './sidebar.js';

const CATEGORIES = [
  { value: 'spam', label: 'Спам' },
  { value: 'abuse', label: 'Оскорбления или угрозы' },
  { value: 'other', label: 'Другое' },
];

export function renderReportMessage(root, handlers) {
  if (!state.reportMessageId) {
    root.innerHTML = '';
    return;
  }

  const category = state.reportMessageCategory || 'spam';

  root.innerHTML = `
    <div class="modal-backdrop" data-action="close-backdrop">
      <div class="modal" data-action="stop-propagation">
        <div class="modal-header">
          <div class="modal-title">Пожаловаться на сообщение</div>
          <button class="modal-close" data-action="close">×</button>
        </div>

        <div class="field">
          <label>Причина</label>
          ${CATEGORIES.map(
            (c) => `
              <label class="report-category-option">
                <input type="radio" name="report-category" value="${c.value}" ${category === c.value ? 'checked' : ''} />
                ${c.label}
              </label>
            `
          ).join('')}
        </div>

        <div class="field">
          <label>Комментарий (необязательно)</label>
          <textarea data-input="report-comment" rows="3" placeholder="Что произошло?">${escapeHtml(state.reportMessageComment || '')}</textarea>
        </div>

        <div class="form-error">${state.reportMessageError || ''}</div>

        <button class="btn-primary" data-action="submit-report" ${state.reportMessageBusy ? 'disabled' : ''}>
          Отправить жалобу
        </button>
      </div>
    </div>
  `;

  root.querySelector('[data-action="close-backdrop"]').addEventListener('click', () => {
    handlers.onClose();
  });
  root.querySelector('[data-action="close"]').addEventListener('click', () => {
    handlers.onClose();
  });
  root.querySelector('.modal').addEventListener('click', (event) => {
    event.stopPropagation();
  });

  root.querySelectorAll('input[name="report-category"]').forEach((input) => {
    input.addEventListener('change', () => {
      state.reportMessageCategory = input.value;
    });
  });

  root.querySelector('[data-action="submit-report"]').addEventListener('click', () => {
    const comment = root.querySelector('[data-input="report-comment"]').value;
    handlers.onSubmit(state.reportMessageCategory || 'spam', comment);
  });
}
