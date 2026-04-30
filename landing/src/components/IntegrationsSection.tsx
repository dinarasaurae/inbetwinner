import React from 'react';
import { motion } from 'framer-motion';
import './IntegrationsSection.css';

const tags = [
  { name: 'Telegram', active: true },
  { name: 'VK', active: true },
  { name: 'Pinterest', active: false },
  { name: 'Facebook', active: false },
  { name: 'Notion', active: true },
  { name: 'Google Calendar', active: true },
  { name: 'Google Sheets', active: true },
  { name: 'OpenAI / Groq', active: true },
  { name: 'Cohere Embeddings', active: true },
  { name: 'Pinecone', active: true },
  { name: 'Instagram (скоро)', active: false },
  { name: 'WhatsApp (скоро)', active: false }
];

const IntegrationsSection: React.FC = () => {
  return (
    <section id="integrations" className="integrations-section">
      <div className="container">
        <motion.div 
          className="section-header"
          initial={{ opacity: 0, y: 20 }}
          whileInView={{ opacity: 1, y: 0 }}
          viewport={{ once: true }}
          transition={{ duration: 0.6 }}
        >
          <h2 className="section-title">Интеграции</h2>
          <p className="section-sub">
            Мессенджеры, CRM и инструменты, с которыми работают агенты
          </p>
        </motion.div>

        <div className="tags-container">
          {tags.map((tag, index) => (
            <motion.div
              key={index}
              className={`integration-tag glass ${tag.active ? 'active' : ''}`}
              initial={{ opacity: 0, scale: 0.8 }}
              whileInView={{ opacity: 1, scale: 1 }}
              viewport={{ once: true }}
              transition={{ duration: 0.4, delay: index * 0.05 }}
              whileHover={{ scale: 1.05 }}
            >
              {tag.name}
            </motion.div>
          ))}
        </div>
      </div>
    </section>
  );
};

export default IntegrationsSection;
