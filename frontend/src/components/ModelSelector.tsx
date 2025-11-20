import React, { useState, useEffect } from 'react';
import { Select, Typography } from 'antd';
import type { SelectProps } from 'antd';
import axios from 'axios';

const { Text } = Typography;

interface ModelSelectorProps {
  value?: string[];
  onChange?: (value: string[]) => void;
  placeholder?: string;
  style?: React.CSSProperties;
}

// 内置常用模型列表
const BUILT_IN_MODELS = [
  // OpenAI GPT 系列
  { label: 'GPT-4o', value: 'gpt-4o', category: 'OpenAI' },
  { label: 'GPT-4o mini', value: 'gpt-4o-mini', category: 'OpenAI' },
  { label: 'GPT-4 Turbo', value: 'gpt-4-turbo', category: 'OpenAI' },
  { label: 'GPT-4 Turbo Preview', value: 'gpt-4-turbo-preview', category: 'OpenAI' },
  { label: 'GPT-4', value: 'gpt-4', category: 'OpenAI' },
  { label: 'GPT-4 32k', value: 'gpt-4-32k', category: 'OpenAI' },
  { label: 'GPT-3.5 Turbo', value: 'gpt-3.5-turbo', category: 'OpenAI' },
  { label: 'GPT-3.5 Turbo 16k', value: 'gpt-3.5-turbo-16k', category: 'OpenAI' },
  { label: 'GPT-3.5 Turbo 1106', value: 'gpt-3.5-turbo-1106', category: 'OpenAI' },
  
  // Anthropic Claude 系列
  { label: 'Claude 3.5 Sonnet (Latest)', value: 'claude-3-5-sonnet-20241022', category: 'Anthropic' },
  { label: 'Claude 3.5 Sonnet', value: 'claude-3-5-sonnet-20240620', category: 'Anthropic' },
  { label: 'Claude 3 Opus', value: 'claude-3-opus-20240229', category: 'Anthropic' },
  { label: 'Claude 3 Sonnet', value: 'claude-3-sonnet-20240229', category: 'Anthropic' },
  { label: 'Claude 3 Haiku', value: 'claude-3-haiku-20240307', category: 'Anthropic' },
  
  // Google Gemini 系列
  { label: 'Gemini 1.5 Pro', value: 'gemini-1.5-pro', category: 'Google' },
  { label: 'Gemini 1.5 Flash', value: 'gemini-1.5-flash', category: 'Google' },
  { label: 'Gemini Pro', value: 'gemini-pro', category: 'Google' },
  { label: 'Gemini Pro Vision', value: 'gemini-pro-vision', category: 'Google' },
  
  // Meta Llama 系列
  { label: 'Llama 3.1 405B', value: 'llama-3.1-405b', category: 'Meta' },
  { label: 'Llama 3.1 70B', value: 'llama-3.1-70b', category: 'Meta' },
  { label: 'Llama 3.1 8B', value: 'llama-3.1-8b', category: 'Meta' },
  { label: 'Llama 3 70B', value: 'llama-3-70b', category: 'Meta' },
  { label: 'Llama 3 8B', value: 'llama-3-8b', category: 'Meta' },
  
  // 阿里通义千问系列
  { label: 'Qwen2.5 72B', value: 'qwen2.5-72b', category: 'Alibaba' },
  { label: 'Qwen2.5 32B', value: 'qwen2.5-32b', category: 'Alibaba' },
  { label: 'Qwen2.5 14B', value: 'qwen2.5-14b', category: 'Alibaba' },
  { label: 'Qwen2.5 7B', value: 'qwen2.5-7b', category: 'Alibaba' },
  { label: 'Qwen3-Coder-Plus', value: 'qwen3-coder-plus', category: 'Alibaba' },
  
  // DeepSeek 系列
  { label: 'DeepSeek Chat', value: 'deepseek-chat', category: 'DeepSeek' },
  { label: 'DeepSeek Coder', value: 'deepseek-coder', category: 'DeepSeek' },
  
  // Mistral 系列
  { label: 'Mistral Large', value: 'mistral-large', category: 'Mistral' },
  { label: 'Mistral Medium', value: 'mistral-medium', category: 'Mistral' },
  { label: 'Mistral Small', value: 'mistral-small', category: 'Mistral' },
];

/**
 * 智能模型选择器组件
 * 
 * 功能特性：
 * 1. 内置常用模型列表（GPT、Claude、Gemini等）
 * 2. 自动获取历史使用过的自定义模型
 * 3. 支持自由输入任意模型名称
 * 4. 按分类组织模型选项
 * 5. 支持批量输入（逗号分隔）
 */
const ModelSelector: React.FC<ModelSelectorProps> = ({ 
  value, 
  onChange, 
  placeholder = '选择常用模型或输入自定义模型名称后按回车...',
  style 
}) => {
  const [historicalModels, setHistoricalModels] = useState<string[]>([]);
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    fetchHistoricalModels();
  }, []);

  // 获取历史使用过的模型
  const fetchHistoricalModels = async () => {
    setLoading(true);
    try {
      // 调用后端API获取所有配置中使用过的模型
      const response = await axios.get('/api/models/history');
      if (response.data.models) {
        setHistoricalModels(response.data.models);
      }
    } catch (error) {
      console.warn('获取历史模型失败，将仅显示内置模型', error);
      setHistoricalModels([]);
    } finally {
      setLoading(false);
    }
  };

  // 生成选项列表
  const generateOptions = (): SelectProps['options'] => {
    const options: SelectProps['options'] = [];
    
    // 1. 添加内置模型（按分类分组）
    const categoriesMap = new Map<string, typeof BUILT_IN_MODELS>();
    BUILT_IN_MODELS.forEach(model => {
      if (!categoriesMap.has(model.category)) {
        categoriesMap.set(model.category, []);
      }
      categoriesMap.get(model.category)!.push(model);
    });
    
    categoriesMap.forEach((models, category) => {
      options.push({
        label: `🏷️ ${category}`,
        title: category,
        options: models.map(m => ({
          label: m.label,
          value: m.value,
        })),
      });
    });
    
    // 2. 添加历史自定义模型（过滤掉已在内置列表中的）
    const builtInValues = new Set(BUILT_IN_MODELS.map(m => m.value));
    const customModels = historicalModels.filter(m => !builtInValues.has(m));
    
    if (customModels.length > 0) {
      options.push({
        label: '📝 历史自定义模型',
        title: '历史自定义模型',
        options: customModels.map(model => ({
          label: `${model} (自定义)`,
          value: model,
        })),
      });
    }
    
    return options;
  };

  return (
    <div>
      <Select
        mode="tags"
        style={{ width: '100%', ...style }}
        placeholder={placeholder}
        value={value}
        onChange={onChange}
        loading={loading}
        tokenSeparators={[',']}
        options={generateOptions()}
        optionFilterProp="label"
        showSearch
        maxTagCount="responsive"
      />
      <Text type="secondary" style={{ fontSize: 12, marginTop: 4, display: 'block' }}>
        💡 提示：可从列表选择常用模型，或直接输入自定义模型名称后按回车添加。支持用逗号分隔批量输入。
      </Text>
    </div>
  );
};

export default ModelSelector;
