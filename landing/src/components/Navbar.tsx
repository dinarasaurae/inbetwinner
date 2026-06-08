import React from 'react';
import { ChevronDown } from 'lucide-react';
import { Button } from './ui/button';

const Navbar: React.FC = () => {
  const openWebApp = () => {
    window.location.href = '/app';
  };

  return (
    <div className="absolute top-0 left-0 right-0 z-50">
      <div className="flex w-full py-5 px-8 items-center justify-between">
        {/* Left: Logo */}
        <div className="flex items-center">
          <img src="/inbetwin-logo.png" alt="inBeTwin" className="h-12 w-auto brightness-0 invert" onError={(e) => {
             // fallback to text if logo not found
             e.currentTarget.style.display = 'none';
             if (e.currentTarget.nextSibling) {
               (e.currentTarget.nextSibling as HTMLElement).style.display = 'block';
             }
          }} />
          <span style={{display: 'none'}} className="font-display italic text-2xl tracking-tight text-hero-heading">inBeTwin</span>
        </div>

        {/* Center: Nav Items */}
        <div className="hidden md:flex items-center gap-1">
          <button className="flex items-center gap-1 text-foreground/90 text-base hover:text-white px-3 py-2 rounded-md transition-colors">
            Возможности <ChevronDown size={16} />
          </button>
          <button className="flex items-center gap-1 text-foreground/90 text-base hover:text-white px-3 py-2 rounded-md transition-colors">
            Решения
          </button>
          <button className="flex items-center gap-1 text-foreground/90 text-base hover:text-white px-3 py-2 rounded-md transition-colors">
            Тарифы
          </button>
          <button className="flex items-center gap-1 text-foreground/90 text-base hover:text-white px-3 py-2 rounded-md transition-colors">
            Обучение <ChevronDown size={16} />
          </button>
        </div>

        {/* Right: Sign Up */}
        <div className="flex items-center">
          <Button variant="heroSecondary" size="sm" onClick={openWebApp}>
            Регистрация
          </Button>
        </div>
      </div>
      {/* Divider */}
      <div className="mt-[3px] w-full h-px bg-gradient-to-r from-transparent via-foreground/20 to-transparent" />
    </div>
  );
};

export default Navbar;
