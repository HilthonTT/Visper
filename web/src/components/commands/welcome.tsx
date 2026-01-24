import {
  Cmd,
  HeroContainer,
  Link,
  PreImg,
  PreName,
  PreNameMobile,
  PreWrapper,
  Seperator,
} from "../styles/weclome-styled";

export const Welcome: React.FC = () => {
  return (
    <HeroContainer data-testid="welcome">
      <div className="info-section">
        <PreName>
          {`        
  _    ___                     
 | |  / (_)________  ___  _____
 | | / / / ___/ __ \\/ _ \\/ ___/
 | |/ / (__  ) /_/ /  __/ /    
 |___/_/____/ .___/\\___/_/     
           /_/                 
          `}
        </PreName>
        <PreWrapper>
          <PreNameMobile>
            {`
  _    ___                
 | |  / (_)__ ___  ___ ____
 | | / / (_-</ _ \\/ -_) __/
 |___/_/___/ .__/\\__/_/   
          /_/             
 
          `}
          </PreNameMobile>
        </PreWrapper>
        <div>Welcome to Visper - Anonymous Real-Time Chat (Version 1.0.0)</div>
        <Seperator>----</Seperator>
        <div>
          Chat freely without registration. Your privacy is our priority.{" "}
          <Link href="https://github.com/yourusername/visper">View source</Link>
          .
        </div>
        <Seperator>----</Seperator>
        <div>
          For a list of available commands, type `<Cmd>help</Cmd>`.
        </div>
      </div>
      <div className="illu-section">
        <PreImg>
          {`
               =====                
            ===========             
          =====     ====            
         ====         ===           
         ===           ===          
         ===           ===  -       
         ===           === -- --    
         ===           ===    -     
      +++++++++++++++++++++++ ---   
      +++++++++++++++++++++++       
      ++++++++-:..::-++++++++       
      ++++++-...:-:...-++++++       
      +++++-....---....-+++++       
      +++++:..:=====:..:+++++       
      +++++=..-=====-..=+++++       
      ++++++=:-=====::=++++++       
      +++++++++=:::=+++++++++       
      +++++++++++++++++++++++       
       +++++++++++++++++++++                          
         `}
        </PreImg>
      </div>
    </HeroContainer>
  );
};
