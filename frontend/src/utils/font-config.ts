// 字体大小配置工具
export interface FontSizeConfig {
  sidebarFontSize: string;
  menuFontSize: string;
  tableFontSize: string;
  tableIconSize: string;
}

// 预设的字体大小配置
export const fontSizePresets = {
  small: {
    sidebarFontSize: '14px',
    menuFontSize: '12px',
    tableFontSize: '11px',
    tableIconSize: '18px'
  },
  medium: {
    sidebarFontSize: '16px',
    menuFontSize: '14px',
    tableFontSize: '12px',
    tableIconSize: '20px'
  },
  large: {
    sidebarFontSize: '18px',
    menuFontSize: '16px',
    tableFontSize: '13px',
    tableIconSize: '22px'
  }
};

// 应用字体大小配置
export const applyFontSizeConfig = (config: FontSizeConfig) => {
  const root = document.documentElement;
  root.style.setProperty('--sidebar-font-size', config.sidebarFontSize);
  root.style.setProperty('--menu-font-size', config.menuFontSize);
  root.style.setProperty('--table-font-size', config.tableFontSize);
  root.style.setProperty('--table-icon-size', config.tableIconSize);
  
  // 保存到本地存储
  localStorage.setItem('font-size-config', JSON.stringify(config));
};

// 从本地存储加载配置
export const loadFontSizeConfig = (): FontSizeConfig => {
  const saved = localStorage.getItem('font-size-config');
  if (saved) {
    try {
      return JSON.parse(saved);
    } catch (e) {
      console.warn('Failed to parse saved font size config');
    }
  }
  return fontSizePresets.medium; // 默认使用中等大小
};

// 初始化字体配置
export const initFontConfig = () => {
  const config = loadFontSizeConfig();
  applyFontSizeConfig(config);
};