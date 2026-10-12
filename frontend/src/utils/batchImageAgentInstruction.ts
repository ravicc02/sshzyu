import type { BatchImageConfigPlatform } from './batchImageConfig'

// Keep the user-provided template verbatim, including its fixed OpenAI parameters.
// Locale and platform must not silently rewrite the text copied to the agent.
export function buildBatchImageAgentInstruction(_locale: string, _platform: BatchImageConfigPlatform): string {
  return `请根据以下 PPT 内容，为每个主题编写一条生图提示词，用于生成对应的整页幻灯片图片。

页面内容：
1. [第 1 页：主题、标题和核心要点]
2. [第 2 页：主题、标题和核心要点]
3. [第 3 页：主题、标题和核心要点]
4. [第 4 页：主题、标题和核心要点]

要求：
- 每条提示词必须以“根据参考图的风格以及背景”开头。
- 明确页面主题、需要出现的文字和内容布局，但不要另外描述视觉风格。
- 使用我提供的内容，不编造数据、产品能力或人物经历。
- 只替换下方格式中的 prompt 占位，不修改末尾参数 4k,16:9,gpt-image-2.5-sunburst。
- 提示词之间使用半角分号 ;。最后一条提示词后，使用半角逗号 , 连接参数；参数之间也使用半角逗号 ,。
- 16:9 中的半角冒号保持原样。每条提示词内部不要使用半角分号，以免被拆成额外页面。
- 只输出一行结构化提示词，不换行，不添加说明，也不要用代码块包裹结果。

输出格式：
prompt1;prompt2;prompt3;prompt4,4K,16:9,gpt-image-2.5-sunburst`
}
