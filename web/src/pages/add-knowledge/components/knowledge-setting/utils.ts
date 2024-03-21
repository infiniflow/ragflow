const getImageName = (prefix: string, length: number) =>
  new Array(length)
    .fill(0)
    .map((x, idx) => `chunk-method/${prefix}-0${idx + 1}`);

export const ImageMap = {
  book: getImageName('book', 4),
  laws: getImageName('law', 4),
  manual: getImageName('manual', 4),
  media: getImageName('media', 2),
  naive: getImageName('naive', 2),
  paper: getImageName('paper', 2),
  presentation: getImageName('presentation', 2),
  qa: getImageName('qa', 2),
  resume: getImageName('resume', 2),
  table: getImageName('table', 2),
};

export const TextMap = {
  book: {
    title: '',
    description: `Supported file formats are docx, excel, pdf, txt.
  Since a book is long and not all the parts are useful, if it's a PDF,
  please setup the page ranges for every book in order eliminate negative effects and save computing time for analyzing.`,
  },
  laws: {
    title: '',
    description: `Supported file formats are docx, pdf, txt.`,
  },
  manual: { title: '', description: `Only pdf is supported.` },
  media: { title: '', description: '' },
  naive: {
    title: '',
    description: `Supported file formats are docx, pdf, txt.
  This method apply the naive ways to chunk files.
  Successive text will be sliced into pieces using 'delimiter'.
  Next, these successive pieces are merge into chunks whose token number is no more than 'Max token number'.`,
  },
  paper: {
    title: '',
    description: `Only pdf is supported.
  The special part is that, the abstract of the paper will be sliced as an entire chunk, and will not be sliced partly.`,
  },
  presentation: {
    title: '',
    description: `The supported file formats are pdf, pptx.
  Every page will be treated as a chunk. And the thumbnail of every page will be stored.
  PPT file will be parsed by using this method automatically, setting-up for every PPT file is not necessary.`,
  },
  qa: {
    title: '',
    description: `Excel and csv(txt) format files are supported.
  If the file is in excel format, there should be 2 column question and answer without header.
  And question column is ahead of answer column.
  And it's O.K if it has multiple sheets as long as the columns are rightly composed.

  If it's in csv format, it should be UTF-8 encoded. Use TAB as delimiter to separate question and answer.

  All the deformed lines will be ignored.
  Every pair of Q&A will be treated as a chunk.`,
  },
  resume: {
    title: '',
    description: `The supported file formats are pdf, docx and txt.`,
  },
  table: {
    title: '',
    description: `Excel and csv(txt) format files are supported.
  For csv or txt file, the delimiter between columns is TAB.
  The first line must be column headers.
  Column headers must be meaningful terms inorder to make our NLP model understanding.
  It's good to enumerate some synonyms using slash '/' to separate, and even better to
  enumerate values using brackets like 'gender/sex(male, female)'.
  Here are some examples for headers:
      1. supplier/vendor\tcolor(yellow, red, brown)\tgender/sex(male, female)\tsize(M,L,XL,XXL)
      2. 姓名/名字\t电话/手机/微信\t最高学历（高中，职高，硕士，本科，博士，初中，中技，中专，专科，专升本，MPA，MBA，EMBA）
  Every row in table will be treated as a chunk.

visual:
  Image files are supported. Video is comming soon.
  If the picture has text in it, OCR is applied to extract the text as a description of it.
  If the text extracted by OCR is not enough, visual LLM is used to get the descriptions.`,
  },
};
