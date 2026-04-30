import React, { Suspense } from 'react';
import AgentNetwork3D from './AgentNetwork3D';

const FeaturesSection: React.FC = () => {
  return (
    <section id="features" className="py-24 relative overflow-hidden bg-background">
      <div className="container max-w-7xl mx-auto px-6">
        
        {/* Header */}
        <div className="text-center mb-16 relative z-10">
          <div className="text-xs text-muted uppercase tracking-[0.3em] mb-4">Архитектура</div>
          <h2 className="text-4xl md:text-6xl font-display font-medium text-foreground mb-6">
            <span className="text-transparent bg-clip-text" style={{ backgroundImage: "linear-gradient(223deg, #E8E8E9 0%, #3A7BBF 104.15%)" }}>Как это работает:</span>
            <br />Оркестрация агентов
          </h2>
          <p className="text-muted-foreground text-lg max-w-2xl mx-auto">
            Один AI Supervisor анализирует сообщение и назначает цепочку из 5 автономных агентов
            для скоринга, RAG-поиска и моментального ответа.
          </p>
        </div>

        {/* 3D R3F Nodes Viewport */}
        <div className="relative w-full h-[600px] md:h-[800px] rounded-[32px] overflow-hidden liquid-glass border border-white/5">
          <Suspense fallback={<div className="w-full h-full flex items-center justify-center text-muted-foreground">Инициализация AI-нодов...</div>}>
            <AgentNetwork3D />
          </Suspense>
          
          <div className="absolute inset-0 pointer-events-none rounded-[32px] shadow-[inset_0_0_50px_rgba(255,255,255,0.02)]"></div>
        </div>

      </div>
    </section>
  );
};

export default FeaturesSection;
