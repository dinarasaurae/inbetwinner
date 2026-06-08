import React, { useState } from 'react';
import { ChevronDown, ChevronUp } from 'lucide-react';
import { motion, AnimatePresence } from 'framer-motion';
import './Footer.css';

const Footer: React.FC = () => {
  const [openSection, setOpenSection] = useState<'tos' | 'privacy' | null>(null);

  const toggleSection = (section: 'tos' | 'privacy') => {
    setOpenSection(openSection === section ? null : section);
  };

  return (
    <footer className="footer">
      <div className="container">
        
        {/* Accordions for Legal Docs */}
        <div className="legal-accordions" id="tos">
          <div className="accordion-item glass">
            <button className="accordion-header" onClick={() => toggleSection('tos')}>
              <h3>Условия использования</h3>
              {openSection === 'tos' ? <ChevronUp /> : <ChevronDown />}
            </button>
            <AnimatePresence>
              {openSection === 'tos' && (
                <motion.div 
                  className="accordion-content"
                  initial={{ height: 0, opacity: 0 }}
                  animate={{ height: 'auto', opacity: 1 }}
                  exit={{ height: 0, opacity: 0 }}
                >
                  <div className="legal-text">
                    <p className="tos-date">Дата вступления в силу: 19 апреля 2026 г.</p>
                    <h4>1. Принятие условий</h4>
                    <p>Используя приложение и сервисы inBeTwin, вы соглашаетесь с настоящими Условиями.</p>
                    <h4>2. Описание сервиса</h4>
                    <p>inBeTwin — платформа автоматизации коммуникаций на базе ИИ.</p>
                    <h4>3. Данные пользователей</h4>
                    <p>Обработка персональных данных осуществляется в соответствии с Политикой конфиденциальности.</p>
                    <p>По всем вопросам: <a href="mailto:legal@inbetwin.ru">legal@inbetwin.ru</a></p>
                  </div>
                </motion.div>
              )}
            </AnimatePresence>
          </div>

          <div className="accordion-item glass" id="privacy">
            <button className="accordion-header" onClick={() => toggleSection('privacy')}>
              <h3>Политика конфиденциальности</h3>
              {openSection === 'privacy' ? <ChevronUp /> : <ChevronDown />}
            </button>
            <AnimatePresence>
              {openSection === 'privacy' && (
                <motion.div 
                  className="accordion-content"
                  initial={{ height: 0, opacity: 0 }}
                  animate={{ height: 'auto', opacity: 1 }}
                  exit={{ height: 0, opacity: 0 }}
                >
                  <div className="legal-text">
                    <p className="tos-date">Дата вступления в силу: 8 июня 2026 г.</p>
                    <h4>1. Какие данные мы собираем</h4>
                    <p>Мы обрабатываем данные аккаунта, настройки подключенных интеграций, сообщения клиентов, лиды, задачи и материалы базы знаний, которые нужны для работы сервиса.</p>
                    <h4>2. Зачем используются данные</h4>
                    <p>Данные используются для авторизации, подключения мессенджеров и CRM, подготовки ответов клиентам, квалификации лидов, обновления карточек в CRM, поддержки пользователей и защиты сервиса.</p>
                    <h4>3. Передача и хранение</h4>
                    <p>Мы не продаем данные пользователей. Доступ к данным ограничен рабочими задачами сервиса, а подключенные платформы, такие как VK, Telegram, Google и CRM-системы, обрабатывают данные по своим правилам.</p>
                    <h4>4. Безопасность</h4>
                    <p>Мы используем технические и организационные меры защиты, ограничиваем доступ к токенам интеграций и рекомендуем подключать только те аккаунты и сообщества, которыми вы вправе управлять.</p>
                    <p>По вопросам обработки данных: <a href="mailto:privacy@inbetwin.ru">privacy@inbetwin.ru</a></p>
                  </div>
                </motion.div>
              )}
            </AnimatePresence>
          </div>
        </div>

        <div className="footer-bottom">
          <div className="footer-logo">
            <img src="/inbetwin-logo.png" alt="inBeTwin" className="h-14 w-auto brightness-0 invert" />
          </div>
          <div className="footer-links">
            <a href="#tos" onClick={(e) => { e.preventDefault(); toggleSection('tos'); }}>Условия</a>
            <a href="#privacy" onClick={(e) => { e.preventDefault(); toggleSection('privacy'); }}>Конфиденциальность</a>
            <a href="mailto:hello@inbetwin.ru">hello@inbetwin.ru</a>
          </div>
          <div className="footer-copy">
            © 2026 inBeTwin. Все права защищены.
          </div>
        </div>
      </div>
    </footer>
  );
};

export default Footer;
