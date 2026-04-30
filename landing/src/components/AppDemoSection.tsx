import React, { useState, useEffect } from 'react';
import { motion, AnimatePresence, useMotionValue, useTransform, useSpring } from 'framer-motion';
import { Zap, Users, Activity } from 'lucide-react';

const chatSequence = [
  { id: 1, type: 'user', text: "Здравствуйте! Подскажите, какие тарифы у вашей платформы для отдела из 15 менеджеров?", delay: 1000 },
  { id: 2, type: 'agent', text: "Добрый день! AI Supervisor проанализировал ваш запрос. Для отдела из 15 человек идеально подойдет тариф 'Enterprise'.", delay: 3000 },
  { id: 3, type: 'system', text: "✨ Lead Score обновлен: 94% (Hot)", delay: 4500 },
  { id: 4, type: 'action', text: "Задача создана в Notion: 'Демо для 15 менеджеров'", delay: 5500 },
];

const AppDemoSection: React.FC = () => {
  const [messages, setMessages] = useState<typeof chatSequence>([]);

  // 3D Interactive Mouse Tracking
  const mouseX = useMotionValue(0.5);
  const mouseY = useMotionValue(0.5);

  const rotateX = useSpring(useTransform(mouseY, [0, 1], [15, -15]), { stiffness: 100, damping: 30 });
  const rotateY = useSpring(useTransform(mouseX, [0, 1], [-15, 15]), { stiffness: 100, damping: 30 });
  const glareX = useTransform(mouseX, [0, 1], [-100, 100]);
  const glareY = useTransform(mouseY, [0, 1], [-100, 100]);

  const handleMouseMove = (e: React.MouseEvent<HTMLDivElement>) => {
    const rect = e.currentTarget.getBoundingClientRect();
    const x = (e.clientX - rect.left) / rect.width;
    const y = (e.clientY - rect.top) / rect.height;
    mouseX.set(x);
    mouseY.set(y);
  };

  const handleMouseLeave = () => {
    mouseX.set(0.5);
    mouseY.set(0.5);
  };

  useEffect(() => {
    let timeouts: ReturnType<typeof setTimeout>[] = [];
    
    const startSequence = () => {
      setMessages([]);
      chatSequence.forEach((msg) => {
        const t = setTimeout(() => {
          setMessages(prev => [...prev, msg]);
        }, msg.delay);
        timeouts.push(t);
      });
    };

    startSequence();
    
    // Loop sequence
    const loopInterval = setInterval(startSequence, 10000);

    return () => {
      timeouts.forEach(clearTimeout);
      clearInterval(loopInterval);
    };
  }, []);

  return (
    <section className="py-24 relative z-10 bg-background overflow-hidden border-t border-white/5">
      <div className="container max-w-7xl mx-auto px-6 flex flex-col lg:flex-row items-center justify-between gap-16">
        
        {/* Left: Text Content */}
        <div className="flex-1 max-w-xl">
          <motion.div 
            initial={{ opacity: 0, y: 30 }}
            whileInView={{ opacity: 1, y: 0 }}
            viewport={{ once: true, margin: "-100px" }}
            transition={{ duration: 0.6 }}
          >
            <h2 className="text-4xl md:text-5xl font-display text-foreground font-bold mb-6">
              Агенты действуют <span className="text-transparent bg-clip-text accent-gradient inline-block">мгновенно</span>
            </h2>
            <p className="text-muted-foreground text-lg leading-relaxed mb-8">
              Вместо ожидания операторов, <strong>AI Supervisor</strong> моментально анализирует контекст и делегирует задачу <strong>Nurturing Agent</strong>'у, который не только отвечает клиенту, но и обновляет CRM в реальном времени.
            </p>
            <ul className="flex flex-col gap-4">
              <li className="flex items-center gap-3 p-4 liquid-glass rounded-xl border border-white/10">
                <Zap className="text-primary" size={20} />
                <span className="text-foreground font-medium text-sm">Обработка 10,000+ диалогов одновременно</span>
              </li>
              <li className="flex items-center gap-3 p-4 liquid-glass rounded-xl border border-white/10">
                <Users className="text-primary" size={20} />
                <span className="text-foreground font-medium text-sm">Автоматический скоринг каждого лида</span>
              </li>
            </ul>
          </motion.div>
        </div>

        {/* Right: Highly realistic Interactive CSS 3D Mockup */}
        <div 
          className="flex-1 relative w-full flex justify-center items-center py-10 perspective-[1200px]"
          onMouseMove={handleMouseMove}
          onMouseLeave={handleMouseLeave}
        >
          {/* Glowing aura behind phone */}
          <motion.div 
            className="absolute top-1/2 left-1/2 w-[300px] h-[500px] bg-primary/20 blur-[120px] rounded-full pointer-events-none" 
            style={{ x: '-50%', y: '-50%' }}
          />
          
          <motion.div 
            style={{ rotateX, rotateY }}
            initial={{ opacity: 0, scale: 0.95 }}
            whileInView={{ opacity: 1, scale: 1 }}
            viewport={{ once: true }}
            transition={{ duration: 0.8 }}
            className="relative w-[340px] h-[680px] rounded-[50px] bg-[#0A0A0B] border-[4px] border-[#2A2A2E] shadow-2xl overflow-hidden origin-center z-10 transform-gpu"
          >
            {/* Edge lighting / Extrusion effect */}
            <div className="absolute inset-0 rounded-[46px] shadow-[inset_0_0_20px_rgba(255,255,255,0.05),inset_0_25px_50px_-12px_rgba(137,170,204,0.15)] pointer-events-none z-50 pointer-events-none" />

            {/* iPhone Dynamic Island */}
            <div className="absolute top-3 left-1/2 -translate-x-1/2 w-[110px] h-[30px] bg-black rounded-full z-50 flex items-center justify-between px-3">
               <div className="w-2 h-2 rounded-full bg-[#111]" />
               <div className="w-2 h-2 rounded-full bg-green-500/50 shadow-[0_0_5px_rgba(34,197,94,0.5)]" />
            </div>

            {/* Screen Content Wrapper */}
            <div className="absolute inset-x-0 top-0 bottom-0 flex flex-col pt-16 pb-8 px-4 bg-gradient-to-b from-[#0F0F12] to-[#050508] overflow-hidden">
               
               {/* Interactive Glass Glare */}
               <motion.div 
                 className="absolute inset-0 pointer-events-none z-40 bg-gradient-to-tr from-transparent via-white/[0.05] to-transparent scale-[2]"
                 style={{ x: glareX, y: glareY }}
               />

               {/* Chat Monitor Header */}
               <div className="flex items-center gap-3 mb-8 pb-4 border-b border-white/5 relative z-20">
                 <div className="w-10 h-10 rounded-full bg-white/5 flex items-center justify-center">
                   <Activity size={18} className="text-green-400" />
                 </div>
                 <div>
                   <div className="text-xs text-foreground font-semibold">AI Supervisor</div>
                   <div className="text-[10px] text-green-400 flex items-center gap-1">
                     <span className="w-1.5 h-1.5 rounded-full bg-green-400 animate-pulse" /> Orchestrating
                   </div>
                 </div>
               </div>

               {/* Chat Area */}
               <div className="flex-1 flex flex-col gap-4 relative z-20">
                  <AnimatePresence>
                    {messages.map((msg) => (
                      <motion.div
                        key={msg.id}
                        initial={{ opacity: 0, y: 10, scale: 0.95 }}
                        animate={{ opacity: 1, y: 0, scale: 1 }}
                        exit={{ opacity: 0, scale: 0.95 }}
                        className={`max-w-[85%] text-sm p-3 rounded-2xl ${
                          msg.type === 'user' 
                            ? 'bg-white/10 text-white rounded-tr-sm self-end backdrop-blur-sm'
                            : msg.type === 'agent'
                            ? 'bg-primary/90 text-white rounded-tl-sm self-start shadow-[0_0_20px_rgba(137,170,204,0.3)] backdrop-blur-sm'
                            : msg.type === 'system'
                            ? 'bg-green-500/10 text-green-400 border border-green-500/20 text-xs w-full text-center rounded-xl self-center backdrop-blur-sm'
                            : 'bg-white/5 text-muted-foreground border border-white/10 text-xs w-full text-center rounded-xl self-center backdrop-blur-sm'
                        }`}
                      >
                        {msg.text}
                      </motion.div>
                    ))}
                  </AnimatePresence>
               </div>
               
               {/* Typing UI */}
               <div className="h-10 rounded-full bg-white/5 border border-white/10 flex items-center px-4 relative z-20">
                 <span className="text-xs text-muted-foreground font-medium">Monitoring memory stream...</span>
               </div>
            </div>
          </motion.div>
        </div>

      </div>
    </section>
  );
};

export default AppDemoSection;
