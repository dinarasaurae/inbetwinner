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
                    <p className="tos-date">Дата вступления в силу: 19 апреля 2026 г.</p>
                    <h4>1. Какие данные мы собираем</h4>
                    <p>Данные учетной записи, мессенджеров, история переписки для AI агентов.</p>
                    <h4>2. Как мы используем данные</h4>
                    <p>Для генерации ответов AI агентами. Мы не продаем данные.</p>
                    <h4>3. Безопасность</h4>
                    <p>AES-256 шифрование, JWT, PostgreSQL в РФ.</p>
                    <p>По вопросам обработки данных: <a href="mailto:privacy@inbetwin.ru">privacy@inbetwin.ru</a></p>
                  </div>
                </motion.div>
              )}
            </AnimatePresence>
          </div>
        </div>

        <div className="footer-bottom">
          <div className="footer-logo">
            in<span className="logo-accent">Be</span>Twin
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
