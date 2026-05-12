export const UserSettingBaseKey = 'user-setting';

export enum UserSettingRouteKey {
  Profile = 'profile',
  Password = 'password',
  Model = 'model',
  System = 'system',
  Api = 'api',
  Team = 'team',
  MCP = 'mcp',
  Logout = 'logout',
}

export const ProfileSettingBaseKey = 'profile-setting';

export enum ProfileSettingRouteKey {
  Profile = 'profile',
  Plan = 'plan',
  Model = 'model',
  System = 'system',
  Api = 'api',
  Team = 'team',
  Prompt = 'prompt',
  Chunk = 'chunk',
  Logout = 'logout',
}

export const TimezoneList = Object.freeze(
  Intl.supportedValuesOf('timeZone')
    .map((tz) => {
      const dtf = new Intl.DateTimeFormat('en-US', {
        hourCycle: 'h24',
        timeZone: tz,
        timeZoneName: 'longOffset',
      });

      const offsetString = dtf.formatToParts(new Date()).at(-1)!.value;
      const match = /^GMT(?<sign>\+|-)(?<hours>\d{2}):(?<minutes>\d{2})$/i.exec(
        offsetString,
      );

      const hours = match?.groups?.hours ?? '00';
      const minutes = match?.groups?.minutes ?? '00';
      const sign = match?.groups?.sign;

      return Object.freeze({
        name: `${offsetString} ${tz}`,
        id: tz,
        offset:
          (sign === '-' ? -1 : 1) * (Number(hours) * 60 + Number(minutes)),
        offsetString,
      });
    })
    .sort((a, b) => a.offset - b.offset),
);

const navigatorTz = new Intl.DateTimeFormat().resolvedOptions().timeZone;
export const DEFAULT_TIMEZONE = TimezoneList.find(
  (tz) => tz.name === navigatorTz,
)!;
