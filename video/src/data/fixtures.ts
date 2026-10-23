/**
 * Synthetic fixtures used on screen. Every value is invented for the film:
 * `.test` mail domains, 199-0000-00xx numbers, fake ids and cities.
 * No real person, account or production record is represented.
 */

export type Column = {
  key: 'id' | 'name' | 'email' | 'mobile' | 'city';
  label: string;
  width: number;
  /** Sensitive columns get scanned + masked. */
  sensitive?: boolean;
  /** Columns the plan leaves untouched. */
  untouched?: boolean;
};

export type Row = {
  id: string;
  name: string;
  email: string;
  mobile: string;
  city: string;
  masked: {
    name: string;
    email: string;
    mobile: string;
  };
  unknown?: boolean;
};

export const COLUMNS: Column[] = [
  {key: 'id', label: 'id', width: 110, untouched: true},
  {key: 'name', label: 'name', width: 210, sensitive: true},
  {key: 'email', label: 'email', width: 400, sensitive: true},
  {key: 'mobile', label: 'mobile', width: 300, sensitive: true},
  {key: 'city', label: 'city', width: 220},
];

export const ROWS: Row[] = [
  {
    id: '001',
    name: '林晓',
    email: 'lin@example.test',
    mobile: '199 0000 0011',
    city: '上海',
    masked: {name: '林**', email: 'l***@example.test', mobile: '199****0011'},
    unknown: true,
  },
  {
    id: '002',
    name: '陈雨',
    email: 'chen@example.test',
    mobile: '199 0000 0012',
    city: '北京',
    masked: {name: '陈**', email: 'c***@example.test', mobile: '199****0012'},
    unknown: true,
  },
  {
    id: '003',
    name: '赵明',
    email: 'zhao@example.test',
    mobile: '199 0000 0013',
    city: '杭州',
    masked: {name: '赵**', email: 'z***@example.test', mobile: '199****0013'},
    unknown: true,
  },
  {
    id: '004',
    name: '周宁',
    email: 'zhou@example.test',
    mobile: '199 0000 0014',
    city: '深圳',
    masked: {name: '周**', email: 'z***@example.test', mobile: '199****0014'},
    unknown: true,
  },
  {
    id: '005',
    name: '吴桐',
    email: 'wu@example.test',
    mobile: '199 0000 0015',
    city: '成都',
    masked: {name: '吴**', email: 'w***@example.test', mobile: '199****0015'},
    unknown: true,
  },
  {
    id: '006',
    name: '郑一',
    email: 'zheng@example.test',
    mobile: '199 0000 0016',
    city: '南京',
    masked: {name: '郑**', email: 'z***@example.test', mobile: '199****0016'},
    unknown: true,
  },
];

/** Five-step CLI workflow shown in Scene 2. */
export const STEPS = [
  {
    key: 'scan',
    label: 'Scan',
    cn: '扫描识别',
    desc: '按字段上下文与格式规则抽样，证据不足保留 UNKNOWN',
    color: '#63dcf3',
  },
  {
    key: 'preview',
    label: 'Preview',
    cn: '脱敏预览',
    desc: '输出处理计划，并证明预览阶段零写入',
    color: '#8fd6ff',
  },
  {
    key: 'approve',
    label: 'Approve',
    cn: '签名审批',
    desc: 'Ed25519 审批记录绑定当前计划摘要',
    color: '#b49cff',
  },
  {
    key: 'apply',
    label: 'Apply',
    cn: '执行脱敏',
    desc: '仅计划字段被替换，批次事务提交',
    color: '#58e0ba',
  },
  {
    key: 'validate',
    label: 'Validate',
    cn: '严格验证',
    desc: '行数、主键、Schema、计划字段与完整性检查',
    color: '#ff996a',
  },
] as const;

/** Detected field evidence, shown in the scan close-up. */
export const FIELD_EVIDENCE = [
  {field: 'email', rule: 'email', verdict: 'SENSITIVE', samples: 6, hit: '100%'},
  {field: 'mobile', rule: 'cn_mobile', verdict: 'SENSITIVE', samples: 6, hit: '100%'},
  {field: 'name', rule: 'cn_person_name', verdict: 'UNKNOWN', samples: 6, hit: '83% < 90%'},
  {field: 'city', rule: 'cn_city', verdict: 'UNKNOWN', samples: 6, hit: '—'},
];

/** Batch plan for the streaming scene. */
export const BATCHES = [
  {id: 1, rows: '1 000', state: 'done'},
  {id: 2, rows: '1 000', state: 'done'},
  {id: 3, rows: '1 000', state: 'done'},
  {id: 4, rows: '1 000', state: 'pending'},
  {id: 5, rows: '1 000', state: 'pending'},
  {id: 6, rows: '1 000', state: 'pending'},
] as const;

export const TOTAL_ROWS = 6000;
