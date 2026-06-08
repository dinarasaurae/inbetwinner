import React, { useRef, useEffect } from 'react';
import gsap from 'gsap';
import { Button } from './ui/button';
import Navbar from './Navbar';

const HeroSection: React.FC = () => {
  const nameRef = useRef<HTMLHeadingElement>(null);
  const blurRef = useRef<HTMLDivElement>(null);
  const openWebApp = () => {
    window.location.href = '/app';
  };

  useEffect(() => {
    // GSAP Entrance Animations
    if (nameRef.current) {
      gsap.fromTo(nameRef.current, 
        { opacity: 0, y: 50 }, 
        { opacity: 1, y: 0, duration: 1.2, delay: 0.1, ease: "power3.out" }
      );
    }
    
    if (blurRef.current) {
      gsap.fromTo(blurRef.current,
        { opacity: 0, y: 20, filter: "blur(10px)" },
        { opacity: 1, y: 0, filter: "blur(0px)", duration: 1, delay: 0.3, ease: "power3.out" }
      );
    }
  }, []);

  return (
    <section className="relative w-full h-screen bg-background overflow-hidden flex flex-col justify-center items-center">
      {/* Dynamic Background Video */}
      <div className="absolute inset-0 z-0 bg-background">
        <video 
          src="/Abstract_Liquid_Glass_Ferrofluid_Motion.mp4"
          autoPlay
          loop
          muted
          playsInline
          className="w-full h-full object-cover object-[center_42%] scale-[1.12] mix-blend-screen"
          style={{ opacity: 0, transition: 'opacity 2s ease-out' }}
          onLoadedData={(e) => { e.currentTarget.style.opacity = '1'; }}
        />
      </div>
      
      {/* Gradient Overlay for text readability */}
      <div className="absolute inset-0 bg-background/40 z-0 pointer-events-none"></div>

      <div className="relative z-50 w-full">
        <Navbar />
      </div>

      <div className="relative z-10 w-full max-w-7xl mx-auto px-6 flex flex-col justify-center flex-1">
        <div className="max-w-4xl pt-10">
          {/* Eyebrow */}
          <div ref={blurRef} className="text-xs text-muted uppercase tracking-[0.3em] mb-6">
            Агенты продаж 2.0
          </div>

          <h1 
            ref={nameRef}
            className="text-6xl md:text-8xl lg:text-[140px] font-normal leading-[0.9] tracking-[-0.04em] mb-8 text-left"
            style={{ fontFamily: "'Geist Sans', sans-serif" }}
          >
            <span className="text-transparent bg-clip-text" style={{ backgroundImage: "linear-gradient(223deg, #E8E8E9 0%, #3A7BBF 104.15%)" }}>Растите</span><br/>
            <span className="font-display italic text-5xl md:text-7xl lg:text-[110px] text-hero-heading leading-[0.9]">через диалоги</span>
          </h1>

          {/* Subtext */}
          <p className="text-hero-sub text-lg leading-8 max-w-md opacity-80 font-body text-left">
            Автономные агенты<br/>
            для обработки B2B лидов
          </p>

          {/* CTA */}
          <div className="mt-10 mb-8 flex justify-start">
            <Button variant="heroSecondary" className="px-[29px] py-[24px]" onClick={openWebApp}>
              Открыть кабинет
            </Button>
          </div>
        </div>
      </div>
      
      {/* Scroll indicator */}
      <div className="absolute bottom-8 left-1/2 -translate-x-1/2 flex flex-col items-center gap-2">
         <span className="text-[10px] text-muted uppercase tracking-[0.2em]">Scroll</span>
         <div className="w-px h-10 bg-stroke relative overflow-hidden">
           <div className="w-full h-full bg-white absolute top-0 animate-scroll-down"></div>
         </div>
      </div>
    </section>
  );
};

export default HeroSection;
