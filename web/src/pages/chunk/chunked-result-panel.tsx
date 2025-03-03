import { ChunkCard } from './chunk-card';
import { ChunkToolbar } from './chunk-toolbar';

const list = new Array(10).fill({
  page: 'page 1',
  content: `Word并不像 TeX／LaTeX为我们提供了合适的定理环境，因此需要我们另想办法。
  
  第1节 自定义定理环境
  我们已经使用了“定理样式”作为定理排版的样式，如：
  
  定理1.1．对顶角相等。
  
  如果大家需要其他的如引理，公理，定义等环境可以仿照定义。
  
  定理1.2．三边对应相等的三角形全等。
  
  我们将这个过程也定义成了宏，在工具栏Theorem里面。书写过程如下：先写好定理本身，然后在该段落处放置光标，打开Theorem工具栏，点SetTheorem，即可见到效果。请尝试下面一个例子：`,
});

export default function ChunkedResultPanel() {
  return (
    <div className="flex-1 py-6 border-l space-y-6">
      <ChunkToolbar text="Chunked  results"></ChunkToolbar>
      <div className="space-y-6  overflow-auto max-h-[87vh] px-9">
        {list.map((x, idx) => (
          <ChunkCard key={idx} content={x.content} activated></ChunkCard>
        ))}
      </div>
    </div>
  );
}
