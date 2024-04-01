const getImageName = (prefix: string, length: number) =>
  new Array(length)
    .fill(0)
    .map((x, idx) => `chunk-method/${prefix}-0${idx + 1}`);

export const ImageMap = {
  book: getImageName('book', 4),
  laws: getImageName('law', 2),
  manual: getImageName('manual', 4),
  picture: getImageName('media', 2),
  naive: getImageName('naive', 2),
  paper: getImageName('paper', 2),
  presentation: getImageName('presentation', 2),
  qa: getImageName('qa', 2),
  resume: getImageName('resume', 2),
  table: getImageName('table', 2),
  one: getImageName('one', 2),
};

export const TextMap = {
  book: {
    title: '',
    description: `<p>Supported file formats are <b>DOCX</b>, <b>PDF</b>, <b>TXT</b>.</p><p>
  Since a book is long and not all the parts are useful, if it's a PDF,
  please setup the <i>page ranges</i> for every book in order eliminate negative effects and save computing time for analyzing.</p>`,
  },
  laws: {
    title: '',
    description: `<p>Supported file formats are <b>DOCX</b>, <b>PDF</b>, <b>TXT</b>.</p><p>
    Legal documents have a very rigorous writing format. We use text feature to detect split point. 
    </p><p>
    The chunk granularity is consistent with 'ARTICLE', and all the upper level text will be included in the chunk.
    </p>`,
  },
  manual: {
    title: '',
    description: `<p>Only <b>PDF</b> is supported.</p><p>
  We assume manual has hierarchical section structure. We use the lowest section titles as pivots to slice documents.
  So, the figures and tables in the same section will not be sliced apart, and chunk size might be large.
  </p>`,
  },
  naive: {
    title: '',
    description: `<p>Supported file formats are <b>DOCX, EXCEL, PPT, IMAGE, PDF, TXT</b>.</p>
  <p>This method apply the naive ways to chunk files: </p>
  <p>
  <li>Successive text will be sliced into pieces using vision detection model.</li>
  <li>Next, these successive pieces are merge into chunks whose token number is no more than 'Token number'.</li></p>`,
  },
  paper: {
    title: '',
    description: `<p>Only <b>PDF</b> file is supported.</p><p>
    If our model works well, the paper will be sliced by it's sections, like <i>abstract, 1.1, 1.2</i>, etc. </p><p>
    The benefit of doing this is that LLM can better summarize the content of relevant sections in the paper, 
    resulting in more comprehensive answers that help readers better understand the paper. 
    The downside is that it increases the context of the LLM conversation and adds computational cost, 
    so during the conversation, you can consider reducing the ‘<b>topN</b>’ setting.</p>`,
  },
  presentation: {
    title: '',
    description: `<p>The supported file formats are <b>PDF</b>, <b>PPTX</b>.</p><p>
  Every page will be treated as a chunk. And the thumbnail of every page will be stored.</p><p>
  <i>All the PPT files you uploaded will be chunked by using this method automatically, setting-up for every PPT file is not necessary.</i></p>`,
  },
  qa: {
    title: '',
    description: `<p><b>EXCEL</b> and <b>CSV/TXT</b> files are supported.</p><p>
  If the file is in excel format, there should be 2 columns question and answer without header.
  And question column is ahead of answer column.
  And it's O.K if it has multiple sheets as long as the columns are rightly composed.</p><p>

  If it's in csv format, it should be UTF-8 encoded. Use TAB as delimiter to separate question and answer.</p><p>

  <i>All the deformed lines will be ignored.
  Every pair of Q&A will be treated as a chunk.</i></p>`,
  },
  resume: {
    title: '',
    description: `<p>The supported file formats are <b>DOCX</b>, <b>PDF</b>, <b>TXT</b>.
    </p><p>
    The résumé comes in a variety of formats, just like a person’s personality, but we often have to organize them into structured data that makes it easy to search.
    </p><p>
    Instead of chunking the résumé, we parse the résumé into structured data. As a HR, you can dump all the résumé you have, 
    the you can list all the candidates that match the qualifications just by talk with <i>'RAGFlow'</i>.
    </p>
    `,
  },
  table: {
    title: '',
    description: `<p><b>EXCEL</b> and <b>CSV/TXT</b> format files are supported.</p><p>
    Here're some tips:
    <ul>
  <li>For csv or txt file, the delimiter between columns is <em><b>TAB</b></em>.</li>
  <li>The first line must be column headers.</li>
  <li>Column headers must be meaningful terms in order to make our LLM understanding.
  It's good to enumerate some synonyms using slash <i>'/'</i> to separate, and even better to
  enumerate values using brackets like <i>'gender/sex(male, female)'</i>.<p>
  Here are some examples for headers:<ol>
      <li>supplier/vendor<b>'TAB'</b>color(yellow, red, brown)<b>'TAB'</b>gender/sex(male, female)<b>'TAB'</b>size(M,L,XL,XXL)</li>
      <li>姓名/名字<b>'TAB'</b>电话/手机/微信<b>'TAB'</b>最高学历（高中，职高，硕士，本科，博士，初中，中技，中专，专科，专升本，MPA，MBA，EMBA）</li>
      </ol>
      </p>
  </li>
  <li>Every row in table will be treated as a chunk.</li>
  </ul>`,
  },
  picture: {
    title: '',
    description: `
  <p>Image files are supported. Video is coming soon.</p><p>
  If the picture has text in it, OCR is applied to extract the text as its text description.
  </p><p>
  If the text extracted by OCR is not enough, visual LLM is used to get the descriptions.
  </p>`,
  },
  one: {
    title: '',
    description: `
  <p>Supported file formats are <b>DOCX, EXCEL, PDF, TXT</b>.
  </p><p>
  For a document, it will be treated as an entire chunk, no split at all.
  </p><p>
  If you want to summarize something that needs all the context of an article and the selected LLM's context length covers the document length, you can try this method.
  </p>`,
  },
};
